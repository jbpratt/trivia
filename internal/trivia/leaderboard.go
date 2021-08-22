package trivia

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/jbpratt/bots/internal/trivia/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed init.sql
var initSql string

//go:embed questions.sql
var questionsSql string

type Leaderboard struct {
	logger *zap.SugaredLogger
	db     *sql.DB
	rw     sync.RWMutex
	rand   *rand.Rand
}

func NewLeaderboard(logger *zap.SugaredLogger, path string) (*Leaderboard, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB(%s): %w", path, err)
	}

	if _, err = db.ExecContext(context.Background(), initSql+questionsSql); err != nil {
		return nil, fmt.Errorf("failed to run init sql: %w", err)
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
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for name, points := range entries {
		var user *models.User
		var exists bool

		exists, err = models.Users(models.UserWhere.Name.EQ(name)).ExistsG(ctx)
		if err != nil {
			return fmt.Errorf("failed to determine if user exists: %w", err)
		}

		if exists {
			user, err = models.Users(models.UserWhere.Name.EQ(name)).OneG(ctx)
			if err != nil {
				return fmt.Errorf("failed to get user(%s): %w", name, err)
			}

			l.logger.Infof("found user to update: %v", user)

			user.Points += int64(points)
			user.GamesPlayed++
			if _, err = user.UpdateG(ctx, boil.Infer()); err != nil {
				return fmt.Errorf("failed to update user: %w", err)
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
				return fmt.Errorf("failed to insert new user: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (l *Leaderboard) Highscores(limit int) (models.UserSlice, error) {
	l.rw.RLock()
	defer l.rw.RUnlock()

	ctx := context.Background()
	if limit == 0 {
		size, err := models.Users().CountG(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get count of users: %w", err)
		}
		limit = int(size)
	}

	return models.Users(
		models.UserWhere.GamesPlayed.GT(0),
		qm.Select(models.UserColumns.Name, models.UserColumns.Points, models.UserColumns.GamesPlayed),
		qm.OrderBy("points desc"),
		qm.Limit(limit),
	).AllG(ctx)
}
