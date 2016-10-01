// Copyright 2016 Peter Goetz
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

package main

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/petergtz/pegomock/pegomock/mockgen"
	"github.com/petergtz/pegomock/pegomock/util"
	"github.com/petergtz/pegomock/pegomock/watch"
)

var (
	app = kingpin.New("pegomock", "Generates mocks based on interfaces.")
)

func main() {
	Run(os.Args, os.Stderr, app, make(chan bool))
}

func Run(cliArgs []string, out io.Writer, app *kingpin.Application, done chan bool) {

	workingDir, err := os.Getwd()
	app.FatalIfError(err, "")

	var (
		generateCmd = app.Command("generate", "Generate mocks based on the args provided. ")
		destination = generateCmd.Flag("output", "Output file; defaults to mock_<interface>_test.go.").Short('o').String()
		packageOut  = generateCmd.Flag("package", "Package of the generated code; defaults to the package from which pegomock was executed suffixed with _test").Default(filepath.Base(workingDir) + "_test").String()
		// TODO: self_package was taken as is from GoMock.
		//       Still don't understand what it's really there for.
		//       So for now it's not tested.
		selfPackage     = generateCmd.Flag("self_package", "If set, the package this mock will be part of.").String()
		debugParser     = generateCmd.Flag("debug", "Print debug information.").Short('d').Bool()
		generateCmdArgs = generateCmd.Arg("args", "A (optional) Go package path + space-separated interface or a .go file").Required().Strings()

		watchCmd           = app.Command("watch", "Watch ")
		watchRecursive     = watchCmd.Flag("recursive", "Recursively watch sub-directories as well.").Short('r').Bool()
		watchUseGoGenerate = watchCmd.Flag("use-go-generate", "Use go generate directive instead of interfaces_to_mock file.").Short('g').Bool()
		watchPackages      = watchCmd.Arg("directories...", "One or more directories of Go packages to watch").Strings()
	)

	app.Writer(out)
	switch kingpin.MustParse(app.Parse(cliArgs[1:])) {

	case generateCmd.FullCommand():
		if err := util.ValidateArgs(*generateCmdArgs); err != nil {
			app.FatalUsage(err.Error())
		}
		sourceArgs, err := util.SourceArgs(*generateCmdArgs)
		if err != nil {
			app.FatalUsage(err.Error())
		}

		mockgen.GenerateMockFileInOutputDir(
			sourceArgs,
			workingDir,
			*destination,
			*packageOut,
			*selfPackage,
			*debugParser,
			out)

	case watchCmd.FullCommand():
		var targetPaths []string
		if len(*watchPackages) == 0 {
			targetPaths = []string{workingDir}
		} else {
			targetPaths = *watchPackages
		}
		if *watchUseGoGenerate {
			util.Ticker(watch.NewDirectoryUpdaterWithGoGenerate(targetPaths, *watchRecursive).Update, 2*time.Second, done)
		} else {
			watch.CreateWellKnownInterfaceListFilesIfNecessary(targetPaths)
			util.Ticker(watch.NewDirectoryUpdaterWithInterfacesToMockFile(targetPaths, *watchRecursive).Update, 2*time.Second, done)
		}
	}
}
