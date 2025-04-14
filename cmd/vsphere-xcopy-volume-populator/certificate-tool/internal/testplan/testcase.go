package testplan

import (
	"certificate-tool/internal/k8s"
	"certificate-tool/internal/utils"
	"certificate-tool/pkg/vmware"
	"context"
	"fmt"
	"github.com/vmware/govmomi"
	"time"

	"github.com/vmware/govmomi/find"
	//"github.com/vmware/govmomi/govmomi"
	"github.com/vmware/govmomi/object"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type TestCaseForPrint struct {
	Name    string                `yaml:"name"`
	Success utils.SuccessCriteria `yaml:"success"`
	VMs     []*utils.VM           `yaml:"vms"`
	Results utils.TestResult      `yaml:"results"`
}

// TestCase defines a single test scenario.
type TestCase struct {
	Name            string                `yaml:"name"`
	Success         utils.SuccessCriteria `yaml:"success"`
	VMs             []*utils.VM           `yaml:"vms"`
	Results         utils.TestResult      `yaml:"results"`
	Namespace       string                `yaml:"-"`
	StorageClass    string                `yaml:"-"`
	ClientSet       *kubernetes.Clientset `yaml:"-"`
	VSphereURL      string                `yaml:"-"`
	VSphereUser     string                `yaml:"-"`
	VSpherePassword string                `yaml:"-"`
	Datacenter      string                `yaml:"-"`
	Datastore       string                `yaml:"-"`
	ResourcePool    string                `yaml:"-"`
	VmdkDownloadURL string                `yaml:"-"`
	LocalVmdkPath   string                `yaml:"-"`
	IsoPath         string                `yaml:"-"`
}

// Run provisions per-pod PVCs, VMs, launches populator pods, and waits.
func (tc *TestCase) Run(ctx context.Context, podImage, vmImage, pvcYamlPath, storageVendorProduct string) error {
	vSphereCtx, cancel, client, finder, dc, ds, rp, err := vmware.SetupVSphere(
		10*time.Minute,
		tc.VSphereURL,
		tc.VSphereUser,
		tc.VSpherePassword,
		tc.Datacenter,
		tc.Datastore,
		tc.ResourcePool,
	)
	if err != nil {
		return fmt.Errorf("vSphere setup failed: %w", err)
	}
	defer cancel()

	// Pass the new VMDK and ISO paths to ensureVMs
	if err := tc.ensureVMs(vSphereCtx, client, finder, dc, ds, rp, tc.Name, vmImage, tc.VMs, tc.VmdkDownloadURL, tc.LocalVmdkPath, tc.IsoPath); err != nil {
		return fmt.Errorf("VM setup failed: %w", err)
	}

	for _, vm := range tc.VMs {
		pvcName := fmt.Sprintf("pvc-%s-%s", tc.Name, vm.NamePrefix)
		if err := k8s.ApplyPVCFromTemplate(tc.ClientSet, tc.Namespace, pvcName, vm.Size, tc.StorageClass, pvcYamlPath); err != nil {
			return fmt.Errorf("failed ensuring PVC %s: %w", pvcName, err)
		}

		podName := fmt.Sprintf("populator-%s-%s", tc.Name, vm.NamePrefix)
		if err := k8s.EnsurePopulatorPod(ctx, tc.ClientSet, tc.Namespace, podName, podImage, tc.Name, *vm, storageVendorProduct, pvcName); err != nil {
			return fmt.Errorf("failed creating pod %s: %w", podName, err)
		}
	}

	newCtx, _ := context.WithTimeout(ctx, 10*time.Minute)
	results, totalTime, err := k8s.PollPodsAndCheck(newCtx, tc.ClientSet, tc.Namespace, fmt.Sprintf("test=%s", tc.Name), tc.Success.MaxTimeSeconds, 5*time.Second, time.Duration(tc.Success.MaxTimeSeconds)*time.Second)
	if err != nil {
		return fmt.Errorf("failed polling pods: %w", err)
	}
	for _, r := range results {
		tc.Results.Success = r.Success
		tc.Results.ElapsedTime = int64(totalTime.Seconds())
		if !r.Success {
			tc.Results.FailureReason = fmt.Sprintln(results)
		}

	}
	return nil
}

// ensureVMs creates VMs and sets their VMDK paths.
func (tc *TestCase) ensureVMs(ctx context.Context, cli *govmomi.Client, finder *find.Finder, dc *object.Datacenter, ds *object.Datastore, rp *object.ResourcePool, testName, vmImage string, vms []*utils.VM, downloadVmdkURL, localVmdkPath, isoPath string) error {
	klog.Infof("Ensuring VMs for test %s", testName)
	for _, vm := range vms {
		fullVMName := fmt.Sprintf("%s-%s", testName, vm.NamePrefix)

		// Use the provided downloadVmdkURL, localVmdkPath, and isoPath
		klog.Infof("Creating VM %s with image %s, VMDK URL: %s, Local VMDK Path: %s, ISO Path: %s", fullVMName, vmImage, downloadVmdkURL, localVmdkPath, isoPath)
		remoteVmdkPath, err := vmware.CreateVM(
			fullVMName,
			tc.VSphereURL,
			tc.VSphereUser,
			tc.VSpherePassword,
			tc.Datacenter,
			tc.Datastore,
			tc.ResourcePool,
			downloadVmdkURL,
			localVmdkPath,
			isoPath,
			10*time.Minute,
		)
		if err != nil {
			return fmt.Errorf("failed to create VM %s: %w", fullVMName, err)
		}
		vm.VmdkPath = remoteVmdkPath
		klog.Infof("VM %s created with VMDK path: %s", fullVMName, vm.VmdkPath)
	}
	return nil
}
