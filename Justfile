set dotenv-filename := ".env.local"

default:
  just --choose

create-migration name:
  migrate create -ext sql -dir migrations -seq -digits 4 {{name}}

migrate-prod *args:
  echo "Migrating $INGEST_DB"
  migrate -path ./migrations -database $INGEST_DB {{args}}
