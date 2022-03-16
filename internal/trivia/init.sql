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
  delimited list of all answers. Unique question allows for `INSERT OR IGNORE`.
*/
CREATE TABLE IF NOT EXISTS questions (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  question_number INTEGER,
  question        TEXT    NOT NULL,
  answer          TEXT    NOT NULL,
  choices         TEXT    NOT NULL,
  source          TEXT    NOT NULL,
  UNIQUE(question)
);

WITH t AS (
  SELECT id, row_number() OVER (ORDER BY random()) AS question_number FROM questions)
UPDATE questions SET question_number = (SELECT t.question_number FROM t WHERE t.id = questions.id);
