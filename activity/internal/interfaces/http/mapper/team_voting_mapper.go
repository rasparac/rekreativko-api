package mapper

import (
	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/domain"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
)

// keepCurrentVote is how "keep current teams" is shown in my_vote
const keepCurrentVote = "keep_current"

// ProposeTeamsRequestToParams converts ProposeTeamsRequest to application params
func ProposeTeamsRequestToParams(req *dtos.ProposeTeamsRequest, sessionID, authorID uuid.UUID) *application.ProposeTeamsParams {
	teams := make([][]uuid.UUID, len(req.Teams))
	for i, team := range req.Teams {
		teams[i] = team.UserIDs
	}

	return &application.ProposeTeamsParams{
		SessionID: sessionID,
		AuthorID:  authorID,
		Teams:     teams,
	}
}

// TeamVotingStateToResponse converts the voting read model to a VotingResponse
// for requesterID (whose vote is my_vote).
func TeamVotingStateToResponse(state *application.TeamVotingState, requesterID uuid.UUID) *dtos.VotingResponse {
	resp := &dtos.VotingResponse{
		VotingStatus:   "none",
		PlayersGoing:   state.PlayersGoing,
		PlayersNeeded:  state.PlayersNeeded,
		EligibleVoters: state.PlayersGoing,
		Items:          []dtos.ProposalResponse{},
	}

	round := state.Round
	if round == nil {
		return resp
	}

	roundID := round.ID()
	teamCount := round.TeamCount()
	startedAt := round.StartedAt()

	resp.RoundID = &roundID
	resp.VotingStatus = string(round.Status())
	resp.CancelledReason = optionalString(string(round.CancelReason()))
	resp.TeamCount = &teamCount
	resp.MinPlayersPerTeam = round.MinPlayersPerTeam()
	resp.VotesCast = len(round.Votes())
	resp.Version = round.Version()
	resp.StartedAt = &startedAt
	resp.EndedAt = round.EndedAt()

	if round.IsOpen() && round.KeepCurrentAllowed() {
		votes := round.VoteCount(domain.KeepCurrentTeams)
		resp.KeepCurrentVotes = &votes
	}

	if choice, voted := round.VoteOf(requesterID); voted {
		myVote := keepCurrentVote
		if choice != domain.KeepCurrentTeams {
			myVote = choice.String()
		}
		resp.MyVote = &myVote
	}

	for _, p := range round.Proposals() {
		teams := make([]dtos.ProposalTeamResponse, 0, len(p.Teams()))
		for position, userIDs := range p.Teams() {
			teams = append(teams, dtos.ProposalTeamResponse{Position: position, UserIDs: userIDs})
		}

		resp.Items = append(resp.Items, dtos.ProposalResponse{
			ID:        p.ID(),
			AuthorID:  p.AuthorID(),
			CreatedAt: p.CreatedAt(),
			Teams:     teams,
			VoteCount: round.VoteCount(p.ID()),
		})
	}

	if round.Status() == domain.VotingStatusClosed {
		resp.Result = &dtos.VotingResultResponse{
			WinnerProposalID: round.WinnerProposalID(),
			KeptCurrent:      round.KeptCurrent(),
		}
	}

	return resp
}
