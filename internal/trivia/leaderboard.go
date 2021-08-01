package trivia

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jbpratt/bots/internal/trivia/leaderboard/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed leaderboard/init.sql
var initSql string

type Leaderboard struct {
	logger *zap.SugaredLogger
	db     *sql.DB
	rw     sync.RWMutex
	rand   *rand.Rand
}

func NewLeaderboard(logger *zap.SugaredLogger, path string) (*Leaderboard, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB(%s): %v", path, err)
	}

	if _, err = db.ExecContext(context.Background(), initSql); err != nil {
		return nil, fmt.Errorf("failed to run init sql: %v", err)
	}

	boil.SetDB(db)
	return &Leaderboard{
		logger: logger,
		db:     db,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func (l *Leaderboard) Update(entries map[string]int) error {
	l.rw.Lock()
	defer l.rw.Unlock()

	l.logger.Info("updating leaderboard with entries: %v", entries)

	ctx := context.Background()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}

	for name, points := range entries {
		var user *models.User
		var exists bool

		exists, err = models.Users(models.UserWhere.Name.EQ(name)).ExistsG(ctx)
		if err != nil {
			return fmt.Errorf("failed to determine if user exists: %v", err)
		}

		if exists {
			user, err = models.Users(models.UserWhere.Name.EQ(name)).OneG(ctx)
			if err != nil {
				return fmt.Errorf("failed to get user(%s): %v", name, err)
			}

			l.logger.Infof("found user to update: %v", user)

			user.Points += int64(points)
			user.GamesPlayed++
			if _, err = user.UpdateG(ctx, boil.Infer()); err != nil {
				return fmt.Errorf("failed to update user: %v", err)
			}
		} else {
			user = &models.User{
				ID:          rand.Int63(),
				Name:        name,
				Points:      int64(points),
				GamesPlayed: 1,
			}
			l.logger.Infof("inserting new user: %v", user)
			if err = user.InsertG(ctx, boil.Infer()); err != nil {
				return fmt.Errorf("failed to insert new user: %v", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func (l *Leaderboard) Highscores(limit int) (models.UserSlice, error) {
	l.rw.RLock()
	defer l.rw.RUnlock()

	return models.Users(
		models.UserWhere.Points.GT(0),
		qm.Select(models.UserColumns.Name, models.UserColumns.Points),
		qm.OrderBy(models.UserColumns.Points),
		qm.Limit(limit),
	).AllG(context.Background())
}
