package runner

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dpastoor/babylon/utils"
)

//FindNextEstDirNum provides the next dir num
func FindNextEstDirNum(modelFile string, dirNames []string, padding int) NextDirSuggestion {
	firstRun := true
	reOrg := false
	existingRunNums := []int{}
	modelNameEst := strings.Join([]string{
		filepath.Base(modelFile),
		"_est_",
	}, "")

	for _, dirName := range dirNames {
		if strings.Contains(dirName, modelNameEst) {
			runNumString := strings.TrimPrefix(dirName, modelNameEst)
			runNum, _ := strconv.Atoi(runNumString)
			existingRunNums = append(existingRunNums, runNum)
		}
	}
	if len(existingRunNums) == 0 {
		return NextDirSuggestion{strings.Join([]string{
			modelNameEst,
			utils.PadNum(1, padding),
		}, ""), false, firstRun}

	}
	// if got here means at least one run currently present
	firstRun = false
	sortedRunNums := sort.IntSlice(existingRunNums)
	sort.Sort(sortedRunNums)
	nextRunNum := sortedRunNums[len(sortedRunNums)-1] + 1

	if nextRunNum != len(sortedRunNums)+1 {
		reOrg = true
	}
	return NextDirSuggestion{strings.Join([]string{
		modelNameEst,
		utils.PadNum(nextRunNum, padding),
	}, ""), reOrg, firstRun}
}
