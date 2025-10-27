package db

// Migrations are handled by tapdb.NewSqliteStore() and tapdb.NewPostgresStore()
// which automatically run migrations on database initialization.
//
// For the lightweight wallet, use InitDatabase() from factory.go which
// delegates to these tapdb constructors.
