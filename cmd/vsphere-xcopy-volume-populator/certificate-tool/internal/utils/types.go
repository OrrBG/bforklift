package utils

// VM represents a VM configuration to be provisioned.
type VM struct {
<<<<<<< HEAD
<<<<<<< HEAD
	NamePrefix string `yaml:"namePrefix"`
	Size       string `yaml:"size"`
	VmdkPath   string `yaml:"vmdkPath"`
=======
	NamePrefix string `yaml:"name_prefix"`
	Size       string `yaml:"size"`
	VmdkPath   string `yaml:"vmkd_path"`
>>>>>>> 233825ac (WIP test plan)
=======
	NamePrefix string `yaml:"namePrefix"`
	Size       string `yaml:"size"`
	VmdkPath   string `yaml:"vmdkPath"`
>>>>>>> 248c3298 (fix yaml parsing)
}

// SuccessCriteria indicates the max allowed run time for a test case.
type SuccessCriteria struct {
<<<<<<< HEAD
<<<<<<< HEAD
	MaxTimeSeconds int `yaml:"maxTimeSeconds"`
=======
	MaxTimeSeconds int `yaml:"max_time_seconds"`
>>>>>>> 233825ac (WIP test plan)
=======
	MaxTimeSeconds int `yaml:"maxTimeSeconds"`
>>>>>>> 248c3298 (fix yaml parsing)
}

// TestResult holds the outcome of a test case.
type TestResult struct {
	Success       bool   `yaml:"success"`
	ElapsedTime   int64  `yaml:"elapsed_time"`
	FailureReason string `yaml:"failure_reason"`
}
