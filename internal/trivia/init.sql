/*
  Store users who have played a game of trivia and their total points
  over the history of playing. Sorting this table by points allows
  for determining an overall leaderboard.
*/
CREATE TABLE IF NOT EXISTS users (
  id           INTEGER NOT NULL PRIMARY KEY,
  name         TEXT    NOT NULL,
  points       INTEGER NOT NULL,
  games_played INTEGER NOT NULL
);

/*
  Store trivia questions scraped from external sources. choices is a comma
  delimited list of all answers and categories is a comma delimited list of
  categories/tags related to the question. Unique question allows for `INSERT
  OR IGNORE`.
*/
CREATE TABLE IF NOT EXISTS questions (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  question     TEXT    NOT NULL,
  answer       TEXT    NOT NULL,
  choices      TEXT    NOT NULL,
  categories   TEXT    NOT NULL,
  used         INTEGER NOT NULL,
  UNIQUE(question)
);
