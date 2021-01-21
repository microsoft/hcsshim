package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/Microsoft/hcsshim/internal/jobobject"
	"github.com/urfave/cli"
)

const (
	cpuLimitFlag     = "cpu-limit"
	cpuWeightFlag    = "cpu-weight"
	memoryLimitFlag  = "memory-limit"
	affinityFlag     = "cpu-affinity"
	useNTVariantFlag = "use-nt"

	usage = `jobobject-util is a command line tool for getting and setting job object limits`
)

func main() {
	app := cli.NewApp()
	app.Name = "jobobject-util"
	app.Commands = []cli.Command{
		getJobObjectLimitsCommand,
		setJobObjectLimitsCommand,
	}
	app.Usage = usage

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var getJobObjectLimitsCommand = cli.Command{
	Name:      "get",
	Usage:     "gets the job object's resource limits",
	ArgsUsage: "get [flags] <name>",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  useNTVariantFlag,
			Usage: `Optional: indicates if the command should use the NT variant of job object Open/Create calls. `,
		},
		cli.BoolFlag{
			Name:  cpuLimitFlag,
			Usage: "Optional: get job object's CPU limit",
		},
		cli.BoolFlag{
			Name:  cpuWeightFlag,
			Usage: "Optional: get job object's CPU weight.",
		},
		cli.BoolFlag{
			Name:  memoryLimitFlag,
			Usage: "Optional: get job object's memory limit in bytes.",
		},
		cli.BoolFlag{
			Name:  affinityFlag,
			Usage: "Optional: get job object's CPU affinity as a bitmask.",
		},
	},
	Action: func(cli *cli.Context) error {
		ctx := context.Background()
		name := cli.Args().First()
		if name == "" {
			return errors.New("`get` command must specify a target job object name")
		}
		options := &jobobject.Options{
			Name:          name,
			Notifications: false,
			UseNTVariant:  cli.Bool(useNTVariantFlag),
		}
		job, err := jobobject.Open(ctx, options)
		if err != nil {
			return err
		}
		defer job.Close()

		output := ""

		// Only allow one processor related flag since limit and weight are
		// mutually exclusive for a job object
		if cli.IsSet(cpuLimitFlag) && cli.IsSet(cpuWeightFlag) {
			return errors.New("cpu limit and weight are mutually exclusive")
		}
		if cli.IsSet(cpuLimitFlag) {
			cpuRate, err := job.GetCPULimit(jobobject.RateBased)
			if err != nil {
				return err
			}
			output += fmt.Sprintf("%s: %d\n", cpuLimitFlag, cpuRate)
		} else if cli.IsSet(cpuWeightFlag) {
			cpuWeight, err := job.GetCPULimit(jobobject.WeightBased)
			if err != nil {
				return err
			}
			output += fmt.Sprintf("%s: %d\n", cpuWeightFlag, cpuWeight)
		}

		if cli.IsSet(memoryLimitFlag) {
			jobObjMemLimit, err := job.GetMemoryLimit()
			if err != nil {
				return err
			}
			output += fmt.Sprintf("%s: %d\n", memoryLimitFlag, jobObjMemLimit)
		}

		if cli.IsSet(affinityFlag) {
			affinity, err := job.GetCPUAffinity()
			if err != nil {
				return err
			}
			affinityString := strconv.FormatUint(affinity, 2)
			output += fmt.Sprintf("%s: %s\n", affinityFlag, affinityString)
		}
		fmt.Fprintln(os.Stdout, output)
		return nil
	},
}

var setJobObjectLimitsCommand = cli.Command{
	Name:      "set",
	Usage:     "tool used to set resource limits on job objects",
	ArgsUsage: "set [flags] <name>",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  useNTVariantFlag,
			Usage: `Optional: indicates if the command should use the NT variant of job object Open/Create calls. `,
		},
		cli.Uint64Flag{
			Name:  cpuLimitFlag,
			Usage: "Optional: set job object's CPU limit",
		},
		cli.Uint64Flag{
			Name:  cpuWeightFlag,
			Usage: "Optional: set job object's CPU weight.",
		},
		cli.Uint64Flag{
			Name:  memoryLimitFlag,
			Usage: "Optional: set job object's memory limit in bytes.",
		},
		cli.StringFlag{
			Name:  affinityFlag,
			Usage: "Optional: set job object's CPU affinity given a bitmask",
		},
	},
	Action: func(cli *cli.Context) error {
		ctx := context.Background()
		name := cli.Args().First()
		if name == "" {
			return errors.New("`set` command must specify a job object name")
		}

		options := &jobobject.Options{
			Name:          name,
			Notifications: false,
			UseNTVariant:  cli.Bool(useNTVariantFlag),
		}
		job, err := jobobject.Open(ctx, options)
		if err != nil {
			return err
		}
		defer job.Close()

		// Only allow one processor related flag since limit and weight are
		// mutually exclusive for a job object
		if cli.IsSet(cpuLimitFlag) && cli.IsSet(cpuWeightFlag) {
			return errors.New("cpu limit and weight are mutually exclusive")
		}
		if cli.IsSet(cpuLimitFlag) {
			cpuRate := uint32(cli.Uint64(cpuLimitFlag))
			if err := job.SetCPULimit(jobobject.RateBased, cpuRate); err != nil {
				return err
			}
		} else if cli.IsSet(cpuWeightFlag) {
			cpuWeight := uint32(cli.Uint64(cpuWeightFlag))
			if err := job.SetCPULimit(jobobject.WeightBased, cpuWeight); err != nil {
				return err
			}
		}

		if cli.IsSet(memoryLimitFlag) {
			memLimitInBytes := cli.Uint64(memoryLimitFlag)
			if err := job.SetMemoryLimit(memLimitInBytes); err != nil {
				return err
			}
		}

		if cli.IsSet(affinityFlag) {
			affinityString := cli.String(affinityFlag)
			affinity, err := strconv.ParseUint(affinityString, 2, 64)
			if err != nil {
				return err
			}
			if err := job.SetCPUAffinity(affinity); err != nil {
				return err
			}
		}

		return nil
	},
}
