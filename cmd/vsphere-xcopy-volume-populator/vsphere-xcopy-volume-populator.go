package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"

	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/ontap"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/populator"
	"github.com/kubev2v/forklift/cmd/vsphere-xcopy-volume-populator/internal/primera3par"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var version = "unknown"

var (
	crName                     string
	crNamespace                string
	pvcSize                    string
	ownerUID                   string
	ownerName                  string
	secretName                 string
	sourceVMDKFile             string
	targetNamespace            string
	storageVendor              string
	storageHostname            string
	storageUsername            string
	storagePassword            string
	storageSkipSSLVerification string
	vsphereHostname            string
	vsphereUsername            string
	vspherePassword            string

	// kube args
	httpEndpoint string
	metricsPath  string
	masterURL    string
	kubeconfig   string

	showVersion bool

	// New flag for host identifier JSON.
	hostIDJson string

	clientSet *kubernetes.Clientset
)

func main() {
	handleArgs()

	// Create the host identifier if provided.
	var hostIdentifier populator.StorageIdentifier
	if hostIDJson != "" {
		var idMap map[string]string
		if err := json.Unmarshal([]byte(hostIDJson), &idMap); err != nil {
			klog.Fatalf("failed to parse host identifier JSON: %v", err)
		}
		// Here, we assume that the host’s “primary” ID is given by the key "iqn" or "wwn".
		primaryIDType := ""
		if _, ok := idMap["iqn"]; ok {
			primaryIDType = "iqn"
		} else if _, ok := idMap["wwn"]; ok {
			primaryIDType = "wwn"
		} else {
			klog.Fatalf("host identifier JSON must contain key 'iqn' or 'wwn'")
		}
		hostIdentifier = populator.StorageIdentifier{
			IDType:  primaryIDType,
			IDValue: idMap,
		}
	} else {
		klog.Infof("No host identifier provided; host mapping will use vendor defaults.")
	}

	var storageApi populator.StorageApi
	switch storageVendor {
	case "ontap":
		sm, err := ontap.NewNetappClonner(storageHostname, storageUsername, storagePassword)
		if err != nil {
			klog.Fatalf("failed to initialize ontap storage mapper: %v", err)
		}
		storageApi = &sm
	case "primera3par":
		// For Primera3Par the clonner now expects a host identifier (of type populator.StorageIdentifier)
		sm, err := primera3par.NewPrimera3ParClonner(storageHostname, storageUsername, storagePassword, storageSkipSSLVerification == "true")
		if err != nil {
			klog.Fatalf("failed to initialize primera3par clonner: %v", err)
		}
		// Example: you might now call EnsureClonnerIgroup passing the hostIdentifier.
		if _, err := sm.EnsureClonnerIgroup("initiator-group", hostIdentifier); err != nil {
			klog.Fatalf("failed to ensure clonner igroup: %v", err)
		}
		storageApi = &sm
	default:
		klog.Fatalf("Unsupported storage vendor %s; use one of [ontap, primera3par]", storageVendor)
	}

	// Validations.
	_, err := populator.ParseVmdkPath(sourceVMDKFile)
	if err != nil {
		klog.Fatal(err)
	}

	p, err := populator.NewWithRemoteEsxcli(storageApi, vsphereHostname, vsphereUsername, vspherePassword)
	if err != nil {
		klog.Fatalf("Failed to create a remote esxcli populator: %v", err)
	}

	volumeHandle, err := getVolumeHandle(clientSet, targetNamespace, ownerName)
	if err != nil {
		klog.Fatalf("Failed to fetch the volume handle details from the target pvc %q: %v", ownerName, err)
	}

	progressCounter, err := setupTracing()
	if err != nil {
		klog.Fatal(err)
	}

	// Channel for progress report.
	progressCh := make(chan int)
	// Channel for quitting with output.
	quitCh := make(chan error)

	go p.Populate(sourceVMDKFile, volumeHandle, progressCh, quitCh)

	for {
		select {
		case p := <-progressCh:
			klog.Infof("progress reported %d", p)
			metric := dto.Metric{}
			if err := progressCounter.WithLabelValues(ownerUID).Write(&metric); err != nil {
				klog.Error(err)
			} else if float64(p) > metric.Counter.GetValue() {
				progressCounter.WithLabelValues(ownerUID).Add(float64(p))
			}
		case q := <-quitCh:
			klog.Infof("channel quit: %v", q)
			if q != nil {
				klog.Fatal(q)
			}
			return
		}
	}
}

func newKubeClient(masterURL, kubeconfig string) (*kubernetes.Clientset, error) {
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	coreCfg := rest.CopyConfig(cfg)
	coreCfg.ContentType = runtime.ContentTypeProtobuf
	return kubernetes.NewForConfig(coreCfg)
}

// getVolumeHandle extracts the volume handle from the PVC.
func getVolumeHandle(kubeClient *kubernetes.Clientset, targetNamespace, targetPVC string) (string, error) {
	pvc, err := kubeClient.CoreV1().PersistentVolumeClaims(targetNamespace).Get(context.Background(), targetPVC, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to fetch the target persistent volume claim %q: %w", pvc.Name, err)
	}
	if pvc.Spec.VolumeName != "" {
		return pvc.Spec.VolumeName, nil
	}

	primePVCName := "prime-" + pvc.GetUID()
	klog.Infof("volume name not found on claim %q; trying prime pvc %q", pvc.Name, primePVCName)
	primePVC, err := kubeClient.CoreV1().PersistentVolumeClaims(targetNamespace).Get(context.Background(), string(primePVCName), metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to fetch the target PVC %q: %w", primePVC.Name, err)
	}
	if primePVC.Spec.VolumeName == "" {
		return "", fmt.Errorf("volume name not found on prime volume claim %q", primePVC.Name)
	}
	return primePVC.Spec.VolumeName, nil
}

func handleArgs() {
	klog.InitFlags(nil)

	// Populator args.
	flag.StringVar(&crName, "cr-name", "", "The Custom Resource name")
	flag.StringVar(&crNamespace, "cr-namespace", "", "The Custom Resource namespace")
	flag.StringVar(&pvcSize, "pvc-size", "", "The size of the PVC (unused)")
	flag.StringVar(&ownerUID, "owner-uid", "", "Owner UID (PVC ID)")
	flag.StringVar(&ownerName, "owner-name", "", "Owner Name (PVC name)")
	flag.StringVar(&secretName, "secret-name", "", "Secret name (for mounting env vars; not for internal use)")
	flag.StringVar(&sourceVMDKFile, "source-vmdk", "", "File name to populate")
	flag.StringVar(&storageVendor, "storage-vendor", "ontap", "Storage vendor; valid values: [ontap, primera3par]")
	flag.StringVar(&targetNamespace, "target-namespace", "", "Namespace of the target PVC")
	flag.StringVar(&storageHostname, "storage-hostname", os.Getenv("STORAGE_HOSTNAME"), "Storage API hostname")
	flag.StringVar(&storageUsername, "storage-username", os.Getenv("STORAGE_USERNAME"), "Storage API username")
	flag.StringVar(&storagePassword, "storage-password", os.Getenv("STORAGE_PASSWORD"), "Storage API password")
	flag.StringVar(&storageSkipSSLVerification, "storage-skip-ssl-verification", os.Getenv("STORAGE_SKIP_SSL_VERIFICATION"), "Skip storage SSL verification")
	flag.StringVar(&vsphereHostname, "vsphere-hostname", os.Getenv("GOVMOMI_HOSTNAME"), "vSphere API hostname")
	flag.StringVar(&vsphereUsername, "vsphere-username", os.Getenv("GOVMOMI_USERNAME"), "vSphere API username")
	flag.StringVar(&vspherePassword, "vsphere-password", os.Getenv("GOVMOMI_PASSWORD"), "vSphere API password")

	// kube flags.
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig (if running out-of-cluster)")
	flag.StringVar(&masterURL, "master", "", "Kubernetes API server address (overrides kubeconfig)")
	// Metrics args.
	flag.StringVar(&httpEndpoint, "http-endpoint", "", "Address for HTTP server (e.g. ':8080')")
	flag.StringVar(&metricsPath, "metrics-path", "/metrics", "HTTP path for Prometheus metrics; default '/metrics'")
	flag.BoolVar(&showVersion, "version", false, "Display version")
	flag.Parse()

	if showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}

	cs, err := newKubeClient(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	clientSet = cs

	missingFlags := false
	flag.VisitAll(func(f *flag.Flag) {
		switch f.Name {
		case "source-vmdk", "target-pvc", "storage-vendor":
			if f.Value.String() == "" {
				missingFlags = true
				klog.Errorf("missing mandatory flag --%s", f.Name)
			}
		case "storage-hostname", "storage-username", "storage-password",
			"vsphere-hostname", "vsphere-username", "vsphere-password":
			if f.Value.String() == "" {
				missingFlags = true
				klog.Errorf("missing flag --%s", f.Name)
			}
		}
	})
	if missingFlags {
		os.Exit(2)
	}
}

func setupTracing() (*prometheus.CounterVec, error) {
	certsDirectory, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		return nil, err
	}

	certBytes, keyBytes, err := cert.GenerateSelfSignedCertKey("", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error generating cert for Prometheus: %v", err)
	}

	certFile := path.Join(certsDirectory, "tls.crt")
	if err = os.WriteFile(certFile, certBytes, 0600); err != nil {
		return nil, fmt.Errorf("error writing cert file: %w", err)
	}

	keyFile := path.Join(certsDirectory, "tls.key")
	if err = os.WriteFile(keyFile, keyBytes, 0600); err != nil {
		return nil, fmt.Errorf("error writing key file: %w", err)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		cfg := tls.Config{MinVersion: tls.VersionTLS12}
		server := http.Server{Addr: ":8443", TLSConfig: &cfg}
		klog.Info("Starting metrics server")
		if err := server.ListenAndServeTLS(certFile, keyFile); err != nil {
			klog.Fatal("Error starting Prometheus endpoint: ", err)
		}
	}()

	progressCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vsphere_xcopy_volume_populator_progress",
		Help: "Progress of vsphere XCOPY volume population",
	}, []string{"ownerUID"})
	if err := prometheus.Register(progressCounter); err != nil {
		return nil, fmt.Errorf("prometheus progress gauge not registered: %w", err)
	}

	return progressCounter, nil
}
