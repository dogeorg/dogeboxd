package core

import (
	"log"
	"os"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
)

type Context struct {
	Config          dogeboxd.ServerConfig
	Enqueue         func(dogeboxd.Action) string
	ActiveJobs      func() ([]dogeboxd.JobRecord, error)
	ReadFile        func(string) ([]byte, error)
	RepoTagsFetcher system.RepoTagsFetcher
}

type Migration struct {
	Name         string
	DisplayName  string
	Version      string
	Requirements func(Context, MigrationRecord) (bool, string, error)
	Run          func(Context, MigrationRecord) (string, bool, error)
}

type RunDecision struct {
	Record     MigrationRecord
	ShouldRun  bool
	SkipReason string
}

func (c Context) ReadFileOrDefault() func(string) ([]byte, error) {
	if c.ReadFile != nil {
		return c.ReadFile
	}

	return os.ReadFile
}

func (c Context) RepoTagsFetcherOrDefault() system.RepoTagsFetcher {
	if c.RepoTagsFetcher != nil {
		return c.RepoTagsFetcher
	}

	return &system.DefaultRepoTagsFetcher{}
}

func (c Context) ActiveJobsOrDefault() func() ([]dogeboxd.JobRecord, error) {
	if c.ActiveJobs != nil {
		return c.ActiveJobs
	}

	return func() ([]dogeboxd.JobRecord, error) {
		return nil, nil
	}
}

func RunMigrations(ctx Context, migrations []Migration) (string, bool, error) {
	for _, migration := range migrations {
		decision, err := EvaluateRunDecision(ctx.Config, migration.Name)
		if err != nil {
			return "", false, err
		}
		if !decision.ShouldRun {
			log.Printf("Skipping %s because %s", migration.DisplayName, decision.SkipReason)
			continue
		}

		record := decision.Record
		if migration.Requirements != nil {
			applies, reason, err := migration.Requirements(ctx, record)
			if err != nil {
				log.Printf("%s check failed: %v", migration.DisplayName, err)
				continue
			}
			if !applies {
				if reason == "" {
					reason = "it does not apply"
				}
				log.Printf("Skipping %s because %s", migration.DisplayName, reason)
				continue
			}
		}

		log.Printf("Running %s", migration.DisplayName)
		jobID, queued, err := migration.Run(ctx, record)
		if err != nil {
			log.Printf("%s failed: %v", migration.DisplayName, err)
			continue
		}
		if !queued {
			continue
		}

		log.Printf("Queued startup %s job: %s", migration.DisplayName, jobID)
		return jobID, true, nil
	}

	return "", false, nil
}

func EvaluateRunDecision(config dogeboxd.ServerConfig, migrationName string) (RunDecision, error) {
	state, err := LoadState(config)
	if err != nil {
		return RunDecision{}, err
	}

	record := state[migrationName]
	if record.RanSuccessfully {
		return RunDecision{
			Record:     record,
			ShouldRun:  false,
			SkipReason: "migration has already run successfully",
		}, nil
	}

	if record.DoNotRun {
		return RunDecision{
			Record:     record,
			ShouldRun:  false,
			SkipReason: "migrations.json has doNotRun=true",
		}, nil
	}

	return RunDecision{
		Record:    record,
		ShouldRun: true,
	}, nil
}
