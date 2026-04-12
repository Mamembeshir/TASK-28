package gamificationservice

import (
	"context"
	"log"
	"time"

	"github.com/eduexchange/eduexchange/internal/model"
	gamificationrepo "github.com/eduexchange/eduexchange/internal/repository/gamification"
	"github.com/google/uuid"
)

// RankingService computes bestseller and new release rankings.
type RankingService struct {
	repo gamificationrepo.GamificationRepository
}

func NewRankingService(repo gamificationrepo.GamificationRepository) *RankingService {
	return &RankingService{repo: repo}
}

const rankingLimit = 20

// GetBestsellers returns the top 20 resources by upvote count this week.
func (s *RankingService) GetBestsellers(ctx context.Context) ([]model.RankingEntry, error) {
	return s.repo.GetBestsellers(ctx, rankingLimit)
}

// GetNewReleases returns the top 20 most recently published this week.
func (s *RankingService) GetNewReleases(ctx context.Context) ([]model.RankingEntry, error) {
	return s.repo.GetNewReleases(ctx, rankingLimit)
}

// WeeklyReset archives current rankings and is called by the cron job on Monday 02:00.
func (s *RankingService) WeeklyReset(ctx context.Context) error {
	now := time.Now()
	year, week := now.ISOWeek()
	// Last week's ISO week
	lastWeek := now.AddDate(0, 0, -7)
	lastYear, lastWeekNum := lastWeek.ISOWeek()

	log.Printf("RankingService.WeeklyReset: archiving week %d/%d", lastYear, lastWeekNum)

	// Archive bestsellers.
	bestsellers, err := s.repo.GetBestsellers(ctx, rankingLimit)
	if err != nil {
		return err
	}
	bsArchive := &model.RankingArchive{
		ID:          uuid.New(),
		WeekNumber:  lastWeekNum,
		Year:        lastYear,
		RankingType: "BESTSELLER",
		Entries:     bestsellers,
	}
	if err := s.repo.CreateRankingArchive(ctx, bsArchive); err != nil {
		log.Printf("RankingService.WeeklyReset: archive bestsellers error: %v", err)
	}

	// Archive new releases.
	releases, err := s.repo.GetNewReleases(ctx, rankingLimit)
	if err != nil {
		return err
	}
	nrArchive := &model.RankingArchive{
		ID:          uuid.New(),
		WeekNumber:  lastWeekNum,
		Year:        lastYear,
		RankingType: "NEW_RELEASE",
		Entries:     releases,
	}
	if err := s.repo.CreateRankingArchive(ctx, nrArchive); err != nil {
		log.Printf("RankingService.WeeklyReset: archive new releases error: %v", err)
	}

	log.Printf("RankingService.WeeklyReset: completed for current week %d/%d", year, week)
	return nil
}

// ListArchives returns past archived rankings.
func (s *RankingService) ListArchives(ctx context.Context, rankingType string) ([]model.RankingArchive, error) {
	return s.repo.ListRankingArchives(ctx, rankingType, 10)
}
