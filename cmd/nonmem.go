// Copyright © 2016 Devin Pastoor <devin.pastoor@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/metrumresearchgroup/babylon/configlib"
	parser "github.com/metrumresearchgroup/babylon/parsers/nmparser"
	"github.com/metrumresearchgroup/babylon/runner"
	"github.com/metrumresearchgroup/babylon/utils"
	"github.com/metrumresearchgroup/turnstile"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//scriptTemplate is a go template we'll use for generating the script to do the work.
const nonMemExecutionTemplate string = `#!/bin/bash

#$ -wd {{ .WorkingDirectory }}

{{ .Command }}
`

//Parse type 2 refers to evenly load balanced work
//Transfer Type 1 refers to MPI
//TIMEOUTI 100 means wait 100 seconds for node to become available
//TIMEOUT 10 means wait 10 seconds for work to complete -> Should default to 10
const nonmemParaFiletemplate string = `$GENERAL
NODES={{ .TotalNodes }} PARSE_TYPE=2 TIMEOUTI=100 TIMEOUT={{ .CompletionTimeout }} PARAPRINT=0 TRANSFER_TYPE=1
$COMMANDS
1: {{ .MpiExecPath }} -wdir "$PWD" -n {{ .HeadNodes }} ./nonmem $*
2:-wdir "$PWD" -n {{ .WorkerNodes }} ./nonmem -wnf
$DIRECTORIES
1:NONE
2-[nodes]:worker{#-1}`

type nonmemParallelDirective struct {
	TotalNodes        int
	CompletionTimeout int
	MpiExecPath       string
	HeadNodes         int
	WorkerNodes       int
}

var controlStreamExtensions []string = []string{
	".mod", //PSN Style
	".ctl", //Metrum Style
}

var nonMemTemporaryFiles []string = []string{
	"background.set",
	"compile.lnk",
	"FCON",
	"FDATA",
	"FMSG",
	"FREPORT",
	"FSIZES",
	"FSTREAM",
	"FSUBS",
	"FSUBS.0",
	"FSUBS.o",
	"FSUBS_MU.F90",
	"FSUBS.f90",
	"fsubs.f90",
	"FSUBS2",
	"gfortran.txt",
	"GFCOMPILE.BAT",
	"INTER",
	"licfile.set",
	"linkc.lnk",
	"LINK.LNK",
	"LINKC.LNK",
	"locfile.set",
	"maxlim.set",
	"newline",
	"nmexec.set",
	"nmpathlist.txt",
	"nmprd4p.mod",
	"nobuild.set",
	"parafile.set",
	"parafprint.set",
	"prcompile.set",
	"prdefault.set",
	"prsame.set",
	"PRSIZES.f90",
	"rundir.set",
	"runpdir.set",
	"simparon.set",
	"temp_dir",
	"tprdefault.set",
	"trskip.set",
	"worker.set",
	"xmloff.set",
	"fort.2001",
	"fort.2002",
	"flushtime.set",
	"nonmem",
	"FPWARN",
	"condorarguments.set",
	"condoropenmpiscript.set",
	"condor.set",
	"mpiloc",
	"nmmpi.sh",
	"temp.out",
	"trashfile.xxx",
}

var parallelRegexesToRemove []string = []string{
	"worker[0-9]{1,}",
	"fort.[0-9]{1,}",
}

//NonMemModel is the definition of a model for NonMem including its target directories and settings required for execution
type NonMemModel struct {
	// BBIVersion is the bbi version used to execute the model
	BBIVersion string `json:"bbi_version"`
	//Model is the name of the model on which we will action: acop.mod
	Model string `json:"model_name"`
	//Path is the Fully Qualified Path to the original model
	Path string `json:"model_path"`
	// DataPath is the path to the data when executing the model
	DataPath string `json:"data_path"`
	// DataMD5 is the md5hash of the data
	DataMD5 string `json:"data_md5"`
	//FileName is the Filename component (sans extension)
	FileName string `json:"model_filename"`
	//Extension is the extension of the file
	Extension string `json:"model_extension"`
	//OriginalPath is the path at which the original model was located: /Users/Documents/acop/
	OriginalPath string `json:"original_path"`
	//OutputDir is the directory into which the copied models and work will be located
	OutputDir string `json:"output_dir"`
	//Settings are basically the cobra definitions / requirements for the iteration
	Configuration *configlib.Config `json:"configuration"`
	//Whether or not the model had an error on generation or execution
	Error error `json:"error"`
}

// RunCmd represents the run command
var nonmemCmd = &cobra.Command{
	Use:   "nonmem",
	Short: "nonmem a (set of) models locally or on the grid",
	Long: `run nonmem model(s), for example: 
bbi nonmem <local|sge> run001.mod
bbi nonmem  --cleanLvl=1 <local|sge> run001.mod run002.mod
bbi nonmem <local|sge> run[001:006].mod // expand to run001.mod run002.mod ... run006.mod local
bbi nonmem <local|sge> .// run all models in directory
 `,
	Run: nonmem,
}

func nonmem(cmd *cobra.Command, args []string) {
	println(runLongDescription)
}

func init() {
	RootCmd.AddCommand(nonmemCmd)

	//NM Selector
	const nmVersionIdentifier string = "nmVersion"
	nonmemCmd.PersistentFlags().String(nmVersionIdentifier, "", "Version of nonmem from the configuration list to use")
	viper.BindPFlag(nmVersionIdentifier, nonmemCmd.PersistentFlags().Lookup(nmVersionIdentifier))

	//Parallelization Components
	const parallelIdentifier string = "parallel"
	nonmemCmd.PersistentFlags().Bool(parallelIdentifier, false, "Whether or not to run nonmem in parallel mode")
	viper.BindPFlag("parallel."+parallelIdentifier, nonmemCmd.PersistentFlags().Lookup(parallelIdentifier))

	const parallelNodesIdentifier string = "nodes"
	nonmemCmd.PersistentFlags().Int(parallelNodesIdentifier, 8, "The number of nodes on which to perform parallel operations")
	viper.BindPFlag("parallel."+parallelNodesIdentifier, nonmemCmd.PersistentFlags().Lookup(parallelNodesIdentifier))

	const parallelCompletionTimeoutIdentifier string = "timeout"
	nonmemCmd.PersistentFlags().Int(parallelCompletionTimeoutIdentifier, 2147483647, "The amount of time to wait for parallel operations in nonmem before timing out")
	viper.BindPFlag("parallel."+parallelCompletionTimeoutIdentifier, nonmemCmd.PersistentFlags().Lookup(parallelCompletionTimeoutIdentifier))

	const mpiExecPathIdentifier string = "mpiExecPath"
	nonmemCmd.PersistentFlags().String(mpiExecPathIdentifier, "/usr/local/mpich3/bin/mpiexec", "The fully qualified path to mpiexec. Used for nonmem parallel operations")
	viper.BindPFlag("parallel."+mpiExecPathIdentifier, nonmemCmd.PersistentFlags().Lookup(mpiExecPathIdentifier))

	const parafileIdentifier string = "parafile"
	nonmemCmd.PersistentFlags().String(parafileIdentifier, "", "Location of a user-provided parafile to use for parallel execution")
	viper.BindPFlag("parallel."+parafileIdentifier, nonmemCmd.PersistentFlags().Lookup(parafileIdentifier))
}

// "Copies" a file by reading its content (optionally updating the path)
func copyFileToDestination(l *NonMemModel, modifyPath bool) error {

	fs := afero.NewOsFs()

	if exists, _ := afero.DirExists(fs, l.OutputDir); !exists {
		//Create the directory
		fs.MkdirAll(l.OutputDir, 0750)
	}

	//Get the lines of the file
	sourceLines, err := utils.ReadLines(l.Path)

	if err != nil {
		return errors.New("Unable to read the contents of " + l.Path)
	}

	//We'll use stats for setting the mode of the target file to make sure perms are the same
	stats, err := fs.Stat(l.Path)

	if err != nil {
		return err
	}

	//If set to modify, let's look for a $DATA line and replace it
	if modifyPath {
		for k, line := range sourceLines {
			if strings.Contains(line, "$DATA") {
				sourceLines[k] = parser.AddPathLevelToData(line)
			}
		}
	}

	//Write the file contents
	fileContents := strings.Join(sourceLines, "\n")

	afero.WriteFile(fs, path.Join(l.OutputDir, l.Model), []byte(fileContents), stats.Mode())

	return nil
}

//processes any template (inlcuding the const one here) to create a byte slice of the entire file
func generateScript(fileTemplate string, l *NonMemModel) ([]byte, error) {

	t, err := template.New("file").Parse(fileTemplate)
	buf := new(bytes.Buffer)
	if err != nil {
		return []byte{}, errors.New("There was an error processing the provided script template")
	}

	type content struct {
		WorkingDirectory string
		Command          string
	}

	err = t.Execute(buf, content{
		WorkingDirectory: l.OutputDir,
		Command:          buildNonMemCommandString(l),
	})

	if err != nil {
		return []byte{}, errors.New("An error occured during the execution of the provided script template")
	}

	if viper.GetBool("debug") {
		log.Debugf("Generated command template for local execution is: %s", buf.String())
	}

	return buf.Bytes(), nil
}

func writeParaFile(l *NonMemModel) error {

	contentBytes, err := generateParaFile(l)

	log.Debugf("Parafile used has contents of : %s", string(contentBytes))

	//Something failed during generation
	if err != nil {
		return err
	}

	var contentLines []string

	//If no parafile is provided, generate one
	if l.Configuration.Parallel.Parafile == "" {
		contentLines = strings.Split(string(contentBytes), "\n")
	} else {
		contentLines, err = utils.ReadLines(l.Configuration.Parallel.Parafile)
		if err != nil {
			log.Fatalf("Unable to read the contents of the parafile provided: %s, Error is %s ", l.Configuration.Parallel.Parafile, err)
		}
	}

	return utils.WriteLines(contentLines, path.Join(l.OutputDir, l.FileName+".pnm"))

}

func generateParaFile(l *NonMemModel) ([]byte, error) {
	nmp := nonmemParallelDirective{
		TotalNodes:        l.Configuration.Parallel.Nodes,
		HeadNodes:         1,
		WorkerNodes:       l.Configuration.Parallel.Nodes - 1,
		CompletionTimeout: l.Configuration.Parallel.Timeout,
		MpiExecPath:       l.Configuration.Parallel.MPIExecPath,
	}

	buf := new(bytes.Buffer)

	t := template.New("parafile")
	parsed, err := t.Parse(nonmemParaFiletemplate)

	if err != nil {
		return []byte{}, err
	}

	err = parsed.Execute(buf, nmp)

	if err != nil {
		return []byte{}, err
	}

	return buf.Bytes(), nil
}

func buildNonMemCommandString(l *NonMemModel) string {

	var nmHome string
	var nmBinary string

	if viper.GetString("nmVersion") == "" {
		//Find the default location
		for _, v := range l.Configuration.Nonmem {
			if v.Default {
				nmHome = v.Home
				nmBinary = v.Executable
			}
		}
	} else {
		//Try to access the Provided value
		if val, ok := l.Configuration.Nonmem[viper.GetString("nmVersion")]; ok {
			nmHome = val.Home
			nmBinary = val.Executable
		} else {
			//Not a valid option!
			log.Fatalf("nmVersion of %s was provided but has no configurations in babylon.yaml!", viper.GetString("nmVersion"))
		}
	}

	if nmHome == "" {
		log.Fatal("No version was supplied and no default value exists in the configset")
	}

	// TODO: Implement cache
	noBuild := false
	nmExecutable := path.Join(nmHome, "run", nmBinary)
	cmdArgs := []string{
		path.Join(l.OutputDir, l.Model),
		"",
		path.Join(l.OutputDir, l.FileName+".lst"),
		"",
	}

	if noBuild {
		cmdArgs = append(cmdArgs, "--nobuild")
	}

	//Section for Appending the parafile command
	if l.Configuration.Parallel.Parallel {
		cmdArgs = append(cmdArgs, "-parafile="+path.Join(l.OutputDir, l.FileName+".pnm"))
	}

	return nmExecutable + " " + strings.Join(cmdArgs, " ")
}

//modelName is the full file + ext representation of the model (ie acop.mod)
//directory is the directory in which we will be performing cleanup
//exceptions is a variadic input for allowing exceptions / overrides.
//We're taking a local model because grid engine execution doesn't wait for completion. Nothing to do :)
func filesToCleanup(model *NonMemModel, exceptions ...string) runner.FileCleanInstruction {
	fci := runner.FileCleanInstruction{
		Location: model.OutputDir,
	}

	files := getCleanableFileList(model.FileName, model.Configuration.CleanLvl)

	for _, v := range files {
		if !isFilenameInExceptions(exceptions, v) {
			//Let's add it to the cleanup list if it's not in the exclusions
			fci.FilesToRemove = append(fci.FilesToRemove, newTargetFile(v, model.Configuration.CleanLvl))
		}
	}

	if model.Configuration.Parallel.Parallel {
		fs := afero.NewOsFs()
		for _, variant := range parallelRegexesToRemove {

			r := regexp.MustCompile(variant)

			files, err := afero.ReadDir(fs, model.OutputDir)

			if err != nil {
				log.Printf("Error trying to read directory %s for parallel files to cleanup", model.OutputDir)
			}

			for _, f := range files {
				if r.MatchString(f.Name()) {
					fci.FilesToRemove = append(fci.FilesToRemove, newTargetFile(f.Name(), model.Configuration.CleanLvl))
				}
			}
		}
	}

	return fci
}

func isFilenameInExceptions(haystack []string, needle string) bool {

	for _, v := range haystack {
		if v == needle {
			return true
		}
	}

	//No matches
	return false
}

func newTargetFile(filename string, level int) runner.TargetedFile {
	return runner.TargetedFile{
		File:  filename,
		Level: level,
	}
}

func filesToCopy(model *NonMemModel, mandatoryFiles ...string) runner.FileCopyInstruction {
	fci := runner.FileCopyInstruction{
		CopyTo:   model.OriginalPath,
		CopyFrom: model.OutputDir,
	}

	//Process mandatory files first
	for _, v := range mandatoryFiles {
		//Crank it up to 11
		fci.FilesToCopy = append(fci.FilesToCopy, newTargetFile(v, 11))
	}

	//Create Target File Entries
	for _, v := range getCopiableFileList(model.Model, model.Configuration.CopyLvl, model.OutputDir) {
		fci.FilesToCopy = append(fci.FilesToCopy, newTargetFile(v, model.Configuration.CopyLvl))
	}

	return fci
}

//Get only te list of files to clean. Separated because of logic requirements
func getCleanableFileList(file string, level int) []string {
	var output []string
	files := make(map[int][]string)

	//These files are files that may be desired above the normal. Classified as "Temp" files. Default removed
	files[1] = nonMemTemporaryFiles

	msfFileSuffixes := []string{
		"",
		"_ETAS",
		"_RMAT",
		"_SMAT",
		".msf",
		"_ETAS.msf",
		"_RMAT.msf",
		"_SMAT.msf",
	}

	for _, f := range msfFileSuffixes {
		files[1] = append(files[1], strings.Replace(fmt.Sprintf("%s%s", file, f), "run", "msfb", 1))
	}

	for i := 0; i <= level; i++ {
		if val, ok := files[i]; ok {
			output = append(output, val...)
		}
	}

	return output
}

func getCopiableFileList(file string, level int, filepath string) []string {
	var output []string
	files := make(map[int][]string)

	//Get the files from the model
	modelPieces := strings.Split(file, ".")
	//Save as even if there's nothing to delimit, the initial value will be the first component
	filename := modelPieces[0]
	if viper.GetBool("debug") {
		log.Infof("%s Attempting to locate copiable files. Provided path is %s", "["+filename+"]", filepath)
	}

	//Explicitly load the file provided by the user
	fileLines, err := utils.ReadLines(path.Join(filepath, file))

	if err != nil {
		//Let the user know this is basically a no-op
		log.Errorf("%s We could not locate or read the mod file (%s) indicated to locate output files. As such, no table or output files will be included in copy / delete operations", "["+filename+"]", filename+".mod")
		log.Errorf("%s Error was specifically : %s", "["+filename+"]", err)
	}

	//Add defined output files to the list at level 1
	files[1] = append(files[1], parser.FindOutputFiles(fileLines)...)

	//Still necessary at th is point to make sure we're adding the level based data (such as the raw output files) to the output slice
	for i := 0; i <= level; i++ {
		if val, ok := files[i]; ok {
			output = append(output, val...)
		}
	}

	//Extensions now
	output = append(output, extrapolateCopyFilesFromExtensions(filename, level, filepath)...)

	return output
}

// Extrapolate extensions into string representations of filenames.
func extrapolateCopyFilesFromExtensions(filename string, level int, filepath string) []string {
	var output []string
	extensions := make(map[int][]string)

	// parser now needs all these files + other tooling uses xml files
	extensions[1] = []string{
		".xml",
		".grd",
		".shk",
		".cor",
		".cov",
		".ext",
		".lst",
	}

	extensions[2] = []string{
		".clt",
		".coi",
		".clt",
		".coi",
		".cpu",
		".shm",
		".phi",
	}

	extensions[3] = []string{
		"",
		"_ETAS",
		"_RMAT",
		"_SMAT",
		".msf",
		"_ETAS.msf",
		"_RMAT.msf",
		"_SMAT.msf",
	}

	//Loop, extrapolate and append to the output slice
	for i := 0; i <= level; i++ {
		if val, ok := extensions[i]; ok {
			for _, ext := range val {
				//For each filename, let's trim and compose the contents
				output = append(output, strings.TrimSpace(fmt.Sprintf("%s%s", filename, ext)))
			}
		}
	}

	return output
}

func newPostWorkInstruction(model *NonMemModel, cleanupExclusions []string, mandatoryCopyFiles []string) runner.PostWorkInstructions {

	return runner.PostWorkInstructions{
		FilesToCopy:  filesToCopy(model, mandatoryCopyFiles...),
		FilesToClean: filesToCleanup(model, cleanupExclusions...),
	}
}

func postWorkNotice(m *turnstile.Manager, t time.Time) {
	log.Debug("Work has completed. Beginning detail display via console")
	if m.Errors > 0 {
		log.Errorf("%d errors were experienced during the run", m.Errors)

		for _, v := range m.ErrorList {
			log.Errorf("Errors were experienced while running model %s. Details are %s", v.RunIdentifier, v.Notes)
		}
	}

	log.Infof("\r%d models completed in %s", m.Completed, time.Since(t))
	println("")
}

//NewNonMemModel creates the core nonmem dataset from the passed arguments
func NewNonMemModel(modelname string) NonMemModel {

	lm := NonMemModel{
		BBIVersion: VERSION,
	}
	fs := afero.NewOsFs()

	if filepath.IsAbs(modelname) {
		lm.Path = modelname
	} else {
		current, err := os.Getwd()
		if err != nil {
			lm.Error = err
		}
		lm.Path = path.Join(current, modelname)
	}

	fi, err := fs.Stat(lm.Path)

	if err != nil {
		return NonMemModel{
			Error: err,
		}
	}

	lm.Model = fi.Name()

	modelPieces := strings.Split(lm.Model, ".")

	//Don't assume the extension will be there. Prep for invalid
	if len(modelPieces) > 1 {
		lm.FileName = modelPieces[0]
		lm.Extension = modelPieces[1]
	} else {
		lm.FileName = lm.Model
		//Leave Extnsion to default
	}

	//Get the raw path of the original by stripping the actual file from it
	lm.OriginalPath = strings.Replace(lm.Path, "/"+lm.Model, "", 1)

	config, err := os.Open(filepath.Join(lm.OriginalPath, "babylon.yaml"))

	if err != nil {
		log.Fatalf("A failure occurred accessing the initial configuration at %s", filepath.Join(lm.OriginalPath, "babylon.yaml"))
	}

	err = viper.ReadConfig(config)

	if err != nil {
		log.Fatalf("Viper had issues parsing the configuration file provided. Details are: %s", err)
	}

	//Process The template from the viper content for output Dir
	t, err := template.New("output").Parse(viper.GetString("outputDir"))
	buf := new(bytes.Buffer)

	if err != nil {
		return NonMemModel{
			Error: err,
		}
	}

	type outputName struct {
		Name string
	}

	//Make sure to only use the filename for the output dir
	err = t.Execute(buf, outputName{
		Name: lm.FileName,
	})

	if err != nil {
		return NonMemModel{
			Error: err,
		}
	}

	//Use the template content plus the original path
	lm.OutputDir = path.Join(lm.OriginalPath, buf.String())

	if err != nil {
		return NonMemModel{
			Error: err,
		}
	}

	lm.Configuration = configlib.UnmarshalViper()

	log.Debug("Contents of configuration are ", lm.Configuration)

	//Check to see if We for some reason have no nonmem contents
	if len(lm.Configuration.Nonmem) == 0 {
		log.Fatal("No nonmem configurations were loaded in from file. Please make sure the nonmem key and its children are present in babylon.yaml")
	}

	return lm
}

func executeNonMemJob(executor func(model *NonMemModel) turnstile.ConcurrentError, model *NonMemModel) turnstile.ConcurrentError {
	return executor(model)
}

func nonmemModelsFromArguments(args []string) []NonMemModel {
	// regex for filename expansion check
	var output []NonMemModel
	AppFs := afero.NewOsFs()
	r := regexp.MustCompile("(.*)?\\[(.*)\\](.*)?")

	//Let's process our args into models
	for _, arg := range args {

		// check if arg is a file or Dir
		// dirty check for if doesn't have an extension is a folder
		_, ext := utils.FileAndExt(arg)
		if ext == "" || arg == "." {
			// could be directory, will need to be careful about the waitgroup as don't want to
			// keep waiting forever since it
			isDir, err := utils.IsDir(arg, AppFs)
			if err != nil || !isDir {
				log.Errorf("issue handling %s, if this is a run please add the extension. Err: (%s)", arg, err)
				continue
			}
			modelsInDir, err := utils.ListModels(arg, ".mod", AppFs)
			if err != nil {
				log.Errorf("issue getting models in dir %s, if this is a run please add the extension. Err: (%s)", arg, err)
				continue
			}
			if viper.GetBool("verbose") || viper.GetBool("debug") {
				log.Debug("adding %v model files in directory %s to queue", len(modelsInDir), arg)
			}

			for _, model := range modelsInDir {
				output = append(output, NewNonMemModel(path.Join(arg, model)))
			}

		} else {
			// figure out if need to do expansion, or run as-is
			if len(r.FindAllStringSubmatch(arg, 1)) > 0 {
				log.Infof("expanding model pattern: %s \n", arg)
				pat, err := utils.ExpandNameSequence(arg)
				if err != nil {
					log.Errorf("err expanding name: %v", err)
					// don't try to run this model
					continue
				}
				if viper.GetBool("verbose") || viper.GetBool("debug") {
					log.Debugf("expanded models: %s \n", pat)
				}
				for _, p := range pat {
					output = append(output, NewNonMemModel(p))
				}
			} else {
				output = append(output, NewNonMemModel(arg))
			}
		}
	}

	return output
}

func doesDirectoryContainOutputFiles(path string, modelname string) bool {
	fs := afero.NewOsFs()

	contents, err := afero.ReadDir(fs, path)

	if err != nil {
		//If we can't read the output, let's indicate that files did exist to trigger an overwrite.
		return true
	}

	contentFiles := getCopiableFileList(modelname, 3, path)

	for _, v := range contents {
		for _, f := range contentFiles {
			if v.Name() == f {
				return true
			}
		}
	}

	return false
}

func (n NonMemModel) LogIdentifier() string {
	return fmt.Sprintf("[%s]", n.FileName)
}

func createChildDirectories(l *NonMemModel, cancel chan bool, channels *turnstile.ChannelMap, sge bool) error {
	fs := afero.NewOsFs()
	log.Debugf("%s Overwrite is currently set to %t", l.LogIdentifier(), viper.GetBool("debug"))
	//Does output directory exist?
	if ok, _ := afero.DirExists(fs, l.OutputDir); ok {
		//If so are we configured to overwrite?
		if l.Configuration.Overwrite {
			log.Debugf("%s Removing directory %s", l.LogIdentifier(), l.OutputDir)
			err := fs.RemoveAll(l.OutputDir)
			if err != nil {
				return err
			}
		}

		if !l.Configuration.Overwrite {
			//If not, we only want to panic if there are nonmem output files in the directory
			if !doesDirectoryContainOutputFiles(l.OutputDir, l.Model) {
				//Continue along if we find no relevant content
				log.Infof("%s No Nonmem output files detected in %s. Good to continue", l.LogIdentifier(), l.OutputDir)
			} else {
				//Or panic because we're in a scenario where we shouldn't purge, but there's content in the directory from previous runs
				log.Debugf("%s Configuration for overwrite was %t, but %s had Nonmem outputs. As such, we will hault operations", l.LogIdentifier(), viper.GetBool("debug"), l.OutputDir)
				return fmt.Errorf("The target directory, %s already exist, but we are configured not to overwrite. Invalid configuration / run state", l.OutputDir)
			}
		}
	}

	//Copy Model into destination and update Data Path
	err := copyFileToDestination(l, true)

	if err != nil {
		return err
	}

	//Now that the directory is created, let's create the gitignore file if specified
	if viper.GetBool("git") {
		log.Debugf("%s Writing initial gitignore file", l.LogIdentifier())
		WriteGitIgnoreFile(l.OutputDir)
	}

	if err != nil {
		RecordConcurrentError(l.Model, fmt.Sprintf("There appears to have been an issue trying to copy %s to %s", l.Model, l.OutputDir), err, channels, cancel)
		return err
	}

	err = configlib.WriteViperConfig(l.OutputDir, sge, l.Configuration)

	if err != nil {
		RecordConcurrentError(l.Model, "Error writing updated viper config", err, channels, cancel)
		return err
	}

	return nil
}
