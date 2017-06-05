// Licensed to Pulumi Corporation ("Pulumi") under one or more
// contributor license agreements.  See the NOTICE file distributed with
// this work for additional information regarding copyright ownership.
// Pulumi licenses this file to You under the Apache License, Version 2.0
// (the "License"); you may not use this file except in compliance with
// the License.  You may obtain a copy of the License at
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
	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/pulumi/lumi/pkg/compiler"
	"github.com/pulumi/lumi/pkg/compiler/core"
	"github.com/pulumi/lumi/pkg/compiler/errors"
	"github.com/pulumi/lumi/pkg/compiler/symbols"
	"github.com/pulumi/lumi/pkg/eval/heapstate"
	"github.com/pulumi/lumi/pkg/eval/rt"
	"github.com/pulumi/lumi/pkg/pack"
	"github.com/pulumi/lumi/pkg/resource"
	"github.com/pulumi/lumi/pkg/tokens"
	"github.com/pulumi/lumi/pkg/util/cmdutil"
)

func NewLumiCmd() *cobra.Command {
	var logFlow bool
	var logToStderr bool
	var verbose int
	cmd := &cobra.Command{
		Use:   "lumi",
		Short: "Lumi is a framework and toolset for reusable stacks of services",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmdutil.InitLogging(logToStderr, verbose, logFlow)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			glog.Flush()
		},
	}

	cmd.PersistentFlags().BoolVar(&logFlow, "logflow", false, "Flow log settings to child processes (like plugins)")
	cmd.PersistentFlags().BoolVar(&logToStderr, "logtostderr", false, "Log to stderr instead of to files")
	cmd.PersistentFlags().IntVarP(
		&verbose, "verbose", "v", 0, "Enable verbose logging (e.g., v=3); anything >3 is very verbose")

	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newDeployCmd())
	cmd.AddCommand(newDestroyCmd())
	cmd.AddCommand(newEnvCmd())
	cmd.AddCommand(newPackCmd())
	cmd.AddCommand(newPlanCmd())
	cmd.AddCommand(newVersionCmd())

	return cmd
}

func prepareCompiler(cmd *cobra.Command, args []string) (compiler.Compiler, *pack.Package) {
	// If there's a --, we need to separate out the command args from the stack args.
	flags := cmd.Flags()
	dashdash := flags.ArgsLenAtDash()
	var packArgs []string
	if dashdash != -1 {
		packArgs = args[dashdash:]
		args = args[0:dashdash]
	}

	// Create a compiler options object and map any flags and arguments to settings on it.
	opts := core.DefaultOptions()
	opts.Args = dashdashArgsToMap(packArgs)

	// If a package argument is present, try to load that package (either via stdin or a path).
	var pkg *pack.Package
	var root string
	if len(args) > 0 {
		pkg, root = readPackageFromArg(args[0])
	}

	// Now create a compiler object based on whether we loaded a package or just have a root to deal with.
	var comp compiler.Compiler
	var err error
	if root == "" {
		comp, err = compiler.Newwd(opts)
	} else {
		comp, err = compiler.New(root, opts)
	}
	if err != nil {
		cmdutil.Sink().Errorf(errors.ErrorCantCreateCompiler, err)
	}

	return comp, pkg
}

// compile just uses the standard logic to parse arguments, options, and to locate/compile a package.  It returns the
// LumiGL graph that is produced, or nil if an error occurred (in which case, we would expect non-0 errors).
func compile(cmd *cobra.Command, args []string, config resource.ConfigMap) *compileResult {
	// Prepare the compiler info and, provided it succeeds, perform the compilation.
	if comp, pkg := prepareCompiler(cmd, args); comp != nil {
		// Create the preexec hook if the config map is non-nil.
		var preexec compiler.Preexec
		configVars := make(map[tokens.Token]*rt.Object)
		if config != nil {
			preexec = config.ConfigApplier(configVars)
		}

		// Now perform the compilation and extract the heap snapshot.
		var heap *heapstate.Heap
		var pkgsym *symbols.Package
		if pkg == nil {
			pkgsym, heap = comp.Compile(preexec)
		} else {
			pkgsym, heap = comp.CompilePackage(pkg, preexec)
		}

		return &compileResult{
			C:          comp,
			Pkg:        pkgsym,
			Heap:       heap,
			ConfigVars: configVars,
		}
	}

	return nil
}

type compileResult struct {
	C          compiler.Compiler
	Pkg        *symbols.Package
	Heap       *heapstate.Heap
	ConfigVars map[tokens.Token]*rt.Object
}