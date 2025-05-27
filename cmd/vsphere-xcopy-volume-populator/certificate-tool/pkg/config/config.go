package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	TestNamespace string `yaml:"test-namespace"`
	Kubeconfig    string `yaml:"kubeconfig"`
	SecretName    string `yaml:"secret-name"`
	PvcYamlPath   string `yaml:"pvc-yaml-path"`
	TestLabels    string `yaml:"test-labels"`

	TestImageLabel     string `yaml:"test-image-label"`
	TestPopulatorImage string `yaml:"test-populator-image"`

	StoragePassword  string `yaml:"storage-password"`
	StorageUser      string `yaml:"storage-user"`
	StorageURL       string `yaml:"storage-url"`
	StorageClassName string `yaml:"storage-class-name"`

	VspherePassword string `yaml:"vsphere-password"`
	VsphereUser     string `yaml:"vsphere-user"`
	VsphereURL      string `yaml:"vsphere-url"`

	VmName                     string `yaml:"vm-name"`
	IsoPath                    string `yaml:"iso-path"`
	DataStore                  string `yaml:"data-store"`
	GuestID                    string `yaml:"guest-id"`
	DataCenter                 string `yaml:"data-center"`
	MemoryMB                   int    `yaml:"memory-mb"`
	CPUs                       int    `yaml:"cpus"`
	Network                    string `yaml:"network"`
	Pool                       string `yaml:"pool"`
	CDDeviceKey                string `yaml:"cd-device-key"`
	GuestUser                  string `yaml:"guest-user"`
	GuestPass                  string `yaml:"guest-pass"`
	DataSizeMB                 int    `yaml:"data-size-mb"`
	WaitTimeout                string `yaml:"wait-timeout"` // Will be parsed to time.Duration
	DownloadVmdkURL            string `yaml:"download-vmdk-url"`
	LocalVmdkPath              string `yaml:"local-vmdk-path"`
	StorageSkipSSLVerification string `yaml:"storage-skip-ssl-verification"`
}

func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func DefaultConfigPath() string {
	return filepath.Join("assets", "config", "static_values.yaml")
}
