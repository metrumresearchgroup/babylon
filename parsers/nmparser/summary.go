package parser

import (
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/logrusorgru/aurora"
	"github.com/olekukonko/tablewriter"
	"github.com/thoas/go-funk"
)

// Summary prints all results from the parsed LstData
func (results ModelOutput) Summary() bool {
	thetaTable := tablewriter.NewWriter(os.Stdout)
	thetaTable.SetAlignment(tablewriter.ALIGN_LEFT)
	thetaTable.SetColWidth(100)
	thetaTable.SetHeader([]string{"Theta", "Name", "Estimate", "StdErr (RSE)"})
	// required for color, prevents newline in row
	thetaTable.SetAutoWrapText(false)

	finalEstimationMethodIndex := len(results.ParametersData) - 1
	for i := range results.ParametersData[finalEstimationMethodIndex].Estimates.Theta {
		numResult := results.ParametersData[finalEstimationMethodIndex].Estimates.Theta[i]
		seResult := results.ParametersData[finalEstimationMethodIndex].StdErr.Theta[i]
		fixed := results.ParametersData[finalEstimationMethodIndex].Fixed.Theta[i]
		var rse float64
		if seResult != -999999999 && numResult != 0 && seResult != DefaultFloat64 && numResult != DefaultFloat64 {
			rse = math.Abs(seResult / numResult * 100)
		}

		var s4 string
		if fixed == 1 {
			s4 = "FIX"
		} else if seResult == -999999999 {
			s4 = "-"
		} else if rse > 30.0 {
			s4 = aurora.Sprintf(aurora.Red("%s"), fmt.Sprintf("%s (%s%%)", strconv.FormatFloat(seResult, 'f', -1, 64), strconv.FormatFloat(rse, 'f', 1, 64)))
		} else {
			s4 = fmt.Sprintf("%s (%s%%)", strconv.FormatFloat(seResult, 'f', -1, 64), strconv.FormatFloat(rse, 'f', 1, 64))
		}

		thetaTable.Append([]string{
			string("TH " + strconv.Itoa(i+1)),
			results.ParameterNames.Theta[i],
			strconv.FormatFloat(numResult, 'f', -1, 64),
			s4})
	}

	omegaTable := tablewriter.NewWriter(os.Stdout)
	omegaTable.SetAlignment(tablewriter.ALIGN_LEFT)
	omegaTable.SetColWidth(100)
	omegaHeaders := []string{"Omega", "Eta", "Estimate"}
	nSubPopsForMixtureModels := len(results.ShrinkageDetails[len(results.RunDetails.EstimationMethod)-1])
	if nSubPopsForMixtureModels > 1 {
		for i := 0; i < nSubPopsForMixtureModels; i++ {
			omegaHeaders = append(omegaHeaders, fmt.Sprintf("Pop%v Shrinkage (%%)", i+1))
		}
	} else {
		// single population
		omegaHeaders = append(omegaHeaders, "Shrinkage (%)")
	}
	omegaTable.SetHeader(omegaHeaders)
	// required for color, prevents newline in row
	thetaTable.SetAutoWrapText(false)

	diagIndices := GetDiagonalIndices(results.ParameterStructures.Omega)
	methodIndex := len(results.RunDetails.EstimationMethod) - 1
	for n := range results.ParametersData[finalEstimationMethodIndex].Estimates.Omega {
		if results.ParameterStructures.Omega[n] == 0 {
			continue
		}
		diagIndex := funk.IndexOfInt(diagIndices, n)
		var shrinkageValues []string
		var etaName string
		val := results.ParametersData[finalEstimationMethodIndex].Estimates.Omega[n]
		if diagIndex > -1 {
			etaName = fmt.Sprintf("ETA%v", diagIndex+1)
			for sp := range results.ShrinkageDetails[methodIndex] {
				if len(results.ShrinkageDetails[methodIndex]) > 0 {
					// get the data for the last method
					shrinkageDetails := results.ShrinkageDetails[len(results.ShrinkageDetails)-1]
					shrinkage := shrinkageDetails[sp].EtaSD[diagIndex]
					if shrinkage > 30.0 {
						shrinkageValues = append(shrinkageValues, aurora.Sprintf(aurora.Red("%s"), fmt.Sprintf("%f", shrinkage)))
					} else {
						shrinkageValues = append(shrinkageValues, fmt.Sprintf("%f", shrinkage))
					}
				}
			}
		}
		omegaTable.Append(append([]string{results.ParameterNames.Omega[n], etaName, fmt.Sprintf("%f", val)}, shrinkageValues...))
	}

	fmt.Println(results.RunDetails.ProblemText)
	fmt.Println("Dataset: " + results.RunDetails.DataSet)
	fmt.Println(fmt.Sprintf("Records: %v   Observations: %v  Patients: %v",
		results.RunDetails.NumberOfDataRecords,
		results.RunDetails.NumberOfObs,
		results.RunDetails.NumberOfPatients,
	))
	fmt.Println("Estimation Method(s):")
	for _, em := range results.RunDetails.EstimationMethod {
		fmt.Println(" - " + em)
	}

	thetaTable.Render()
	omegaTable.Render()
	return true
}
