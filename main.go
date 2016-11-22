package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dpastoor/nonmemutils/parser"
)

func main() {
	data, _ := readLines("parser/fixtures/lstfiles/simple-onecmpt-ex1.lst")
	results := parser.ParseLstEstimationFile(data)
	bs, _ := json.MarshalIndent(results, "", "\t")
	fmt.Println(string(bs))
	results.Summary()
}

func readLines(path string) ([]string, error) {
	inFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer inFile.Close()
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, nil
}
