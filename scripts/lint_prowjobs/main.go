/*
Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"k8s.io/test-infra/prow/config"
	yaml "sigs.k8s.io/yaml"
)

type JobConstants struct {
	Bucket             string
	Cluster            string
	ServiceAccountName string
	DefaultMakeTarget  string
}

type UnmarshaledJobConfig struct {
	GithubRepo    string
	FileName      string
	FileContents  string
	ProwjobConfig *config.JobConfig
}

type presubmitCheck func(presubmitConfig config.Presubmit, fileContentsString string) (passed bool, lineNo int, errorMessage string)

func (jc *JobConstants) Init() {
	jc.Bucket = "s3://prowpresubmitsdataclusterstack-prowbucket7c73355c-vfwwxd2eb4gp"
	jc.Cluster = "prow-presubmits-cluster"
	jc.ServiceAccountName = "presubmits-build-account"
	jc.DefaultMakeTarget = "build"
}

func findLineNumber(fileContentsString string, searchString string) int {
	fileLines := strings.Split(fileContentsString, "\n")

	for lineNo, fileLine := range fileLines {
		if strings.Contains(fileLine, searchString) {
			return lineNo + 1
		}
	}

	return 0
}

func AlwaysRunCheck() presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if presubmitConfig.AlwaysRun {
			return false, findLineNumber(fileContentsString, "always_run:"), "Please set always_run to false"
		}
		return true, 0, ""
	})
}

func SkipReportCheck() presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if presubmitConfig.Reporter.SkipReport {
			return false, findLineNumber(fileContentsString, "skip_report:"), "Please set always_run to false"
		}
		return true, 0, ""
	})
}

func BucketCheck(jc *JobConstants) presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if presubmitConfig.JobBase.UtilityConfig.DecorationConfig.GCSConfiguration.Bucket != jc.Bucket {
			return false, findLineNumber(fileContentsString, "bucket:"), fmt.Sprintf(`Incorrect bucket configuration, please configure S3 bucket as => bucket: %s`, jc.Bucket)
		}
		return true, 0, ""
	})
}

func ClusterCheck(jc *JobConstants) presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if presubmitConfig.JobBase.Cluster != jc.Cluster {
			return false, findLineNumber(fileContentsString, "cluster:"), fmt.Sprintf(`Incorrect cluster configuration, please configure cluster as => cluster: "%s"`, jc.Cluster)
		}
		return true, 0, ""
	})
}

func ServiceAccountCheck(jc *JobConstants) presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if strings.Contains(presubmitConfig.JobBase.Name, "e2e") {
			return true, 0, ""
		}
		if presubmitConfig.JobBase.Spec.ServiceAccountName != jc.ServiceAccountName {
			return false, findLineNumber(fileContentsString, "serviceaccountName:"), fmt.Sprintf(`Incorrect service account configuration, please configure service account as => serviceaccountName: %s`, jc.ServiceAccountName)
		}
		return true, 0, ""
	})
}

func MakeTargetCheck(jc *JobConstants) presubmitCheck {
	return presubmitCheck(func(presubmitConfig config.Presubmit, fileContentsString string) (bool, int, string) {
		if strings.Contains(presubmitConfig.JobBase.Name, "e2e") || strings.Contains(presubmitConfig.JobBase.Name, "lint") || strings.Contains(presubmitConfig.JobBase.Name, "mocks") || presubmitConfig.JobBase.Name == "eks-anywhere-attribution-files-presubmit" {
			return true, 0, ""
		}
		jobMakeTargetMatches := regexp.MustCompile(`make (\w+[-\w]+?).*`).FindStringSubmatch(strings.Join(presubmitConfig.JobBase.Spec.Containers[0].Command, " "))
		jobMakeTarget := jobMakeTargetMatches[len(jobMakeTargetMatches)-1]
		makeCommandLineNo := findLineNumber(fileContentsString, "make")
		if jobMakeTarget != jc.DefaultMakeTarget {
			return false, makeCommandLineNo, fmt.Sprintf(`Invalid make target, please use the "%s" target`, jc.DefaultMakeTarget)
		}
		return true, 0, ""
	})
}

func getFilesChanged(gitRoot string, pullBaseSha string, pullPullSha string) ([]string, error) {
	presubmitFiles := []string{}
	gitDiffCommand := []string{"git", "-C", gitRoot, "diff", "--name-only", pullBaseSha, pullPullSha}
	fmt.Println("\n", strings.Join(gitDiffCommand, " "))

	gitDiffOutput, err := exec.Command("git", gitDiffCommand[1:]...).Output()

	filesChanged := strings.Fields(string(gitDiffOutput))
	for _, file := range filesChanged {
		if strings.Contains(file, "presubmits") {
			presubmitFiles = append(presubmitFiles, file)
		}
	}
	return presubmitFiles, err
}

func unmarshalJobFile(filePath string, jobConfig *config.JobConfig) (*UnmarshaledJobConfig, error, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil, nil
	}
	unmarshaledJobConfig := new(UnmarshaledJobConfig)
	unmarshaledJobConfig.GithubRepo = strings.Replace(filepath.Dir(filePath), "jobs/", "", 1)
	unmarshaledJobConfig.FileName = filepath.Base(filePath)

	fileContents, fileReadError := ioutil.ReadFile(filePath)
	unmarshaledJobConfig.FileContents = string(fileContents)

	fileUnmarshalError := yaml.Unmarshal(fileContents, &jobConfig)
	unmarshaledJobConfig.ProwjobConfig = jobConfig

	return unmarshaledJobConfig, fileReadError, fileUnmarshalError
}

func displayConfigErrors(fileErrorMap map[string][]string) bool {
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	errorsExist := false
	for file, configErrors := range fileErrorMap {
		if len(configErrors) > 0 {
			errorsExist = true
			fmt.Println()
			fmt.Printf("\n%s:\n", file)
			for _, errorString := range configErrors {
				fmt.Fprintln(w, errorString)
			}
		}
	}
	w.Flush()
	return errorsExist
}

func main() {
	var jobConfig config.JobConfig
	var presubmitErrors = make(map[string][]string)

	presubmitConstants := new(JobConstants)
	presubmitConstants.Init()

	pullBaseSha := os.Getenv("PULL_BASE_SHA")
	pullPullSha := os.Getenv("PULL_PULL_SHA")

	gitRootOutput, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		log.Fatalf("There was an error running the git command: %v", err)
	}
	gitRoot := strings.Fields(string(gitRootOutput))[0]

	presubmitFiles, err := getFilesChanged(gitRoot, pullBaseSha, pullPullSha)
	if err != nil {
		log.Fatalf("There was an error running the git command!")
	}

	presubmitCheckFunctions := []presubmitCheck{
		AlwaysRunCheck(),
		ClusterCheck(presubmitConstants),
		SkipReportCheck(),
		BucketCheck(presubmitConstants),
		ServiceAccountCheck(presubmitConstants),
		MakeTargetCheck(presubmitConstants),
	}

	for _, presubmitFile := range presubmitFiles {
		unmarshaledJobConfig, fileReadError, fileUnmarshalError := unmarshalJobFile(presubmitFile, &jobConfig)
		// Skip linting if file is not found
		if unmarshaledJobConfig == nil {
			continue
		}
		if fileReadError != nil {
			log.Fatalf("Error reading contents of %s: %v", presubmitFile, fileReadError)
		}
		if fileUnmarshalError != nil {
			log.Fatalf("Error unmarshaling contents of %s: %v", presubmitFile, fileUnmarshalError)
		}

		presubmitConfigs, ok := unmarshaledJobConfig.ProwjobConfig.PresubmitsStatic[unmarshaledJobConfig.GithubRepo]
		if !ok {
			log.Fatalf("Key %s does not exist in Presubmit configuration map", unmarshaledJobConfig.GithubRepo)
		}
		if len(presubmitConfigs) < 1 {
			log.Fatalf("Presubmit configuration for the %s repo is empty", unmarshaledJobConfig.GithubRepo)
		}
		presubmitConfig := presubmitConfigs[0]
		for _, check := range presubmitCheckFunctions {
			passed, lineNum, errMessage := check(presubmitConfig, unmarshaledJobConfig.FileContents)
			if !passed {
				errorString := fmt.Sprintf("%d\t%s", lineNum, errMessage)
				presubmitErrors[unmarshaledJobConfig.FileName] = append(presubmitErrors[unmarshaledJobConfig.FileName], errorString)
			}
		}
	}

	presubmitErrorsExist := displayConfigErrors(presubmitErrors)

	if presubmitErrorsExist {
		fmt.Println("❌ Validations failed!")
		os.Exit(1)
	}
	fmt.Println("✅ Validations passed!")
}
