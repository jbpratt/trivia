/*
  Store users who have played a game of trivia and their total points
  over the history of playing. Sorting this table by points allows
  for determining an overall leaderboard.
*/
CREATE TABLE IF NOT EXISTS users (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT NOT NULL,
  points       INTEGER NOT NULL,
  games_played INTEGER NOT NULL
);
