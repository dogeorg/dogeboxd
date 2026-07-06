package migrations

import (
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/definitions"
)

func postUpgradeMigrations() []core.Migration {
	return []core.Migration{
		definitions.OSFlakeMigration,
	}
}
