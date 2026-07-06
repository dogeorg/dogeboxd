package migrations

import "github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"

type Context = core.Context

func RunPostUpgradeMigrations(ctx Context) (string, bool, error) {
	return core.RunMigrations(ctx, postUpgradeMigrations())
}
