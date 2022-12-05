package jobs

import (
	"fmt"

	"github.com/aws/eks-distro-prow-jobs/templater/jobs/types"
	"github.com/aws/eks-distro-prow-jobs/templater/jobs/utils"
)

func GetJobList(jobType string) (map[string]map[string]types.JobConfig, error) {
	switch jobType {
	case "periodic":
		repos := []string{"eks-anywhere", "eks-anywhere-build-tooling"}
		periodicsList, err := utils.GetJobsByType(repos, "periodic")
		if err != nil {
			return nil, fmt.Errorf("error getting periodic list:%v", err)
		}
		return periodicsList, nil
	case "postsubmit":
		repos := []string{"eks-anywhere"}
		postsubmitsList, err := utils.GetJobsByType(repos, "postsubmit")
		if err != nil {
			return nil, fmt.Errorf("error getting postsubmits list:%v", err)
		}
		return postsubmitsList, nil
	case "presubmit":
		repos := []string{"eks-anywhere", "eks-anywhere-build-tooling", "eks-anywhere-packages", "eks-anywhere-prow-jobs"}
		presubmitsList, err := utils.GetJobsByType(repos, "presubmit")
		if err != nil {
			return nil, fmt.Errorf("error getting presubmits list:%v", err)
		}
		return presubmitsList, nil
	default:
		return nil, fmt.Errorf("Unsupported job type: %s", jobType)
	}
}
