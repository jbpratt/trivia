#!/usr/bin/env bash

python misc/gen-sql.py

pushd internal/trivia
rm -f trivia.db
sqlite3 trivia.db ".read init.sql" ".read questions.sql" ".exit"
sqlboiler -c sqlboiler.toml --wipe sqlite3
# rm trivia.db
popd
