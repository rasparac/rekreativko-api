package http

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/rasparac/rekreativko-api/activity/internal/application"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/dtos"
	"github.com/rasparac/rekreativko-api/activity/internal/interfaces/http/mapper"
	"github.com/rasparac/rekreativko-api/shared/api"
	"github.com/rasparac/rekreativko-api/shared/authcontext"
)

// ProposeTeams handles POST /api/v1/sessions/{id}/proposals
//
//	@Summary		Propose a division into teams
//	@Description	Anyone going proposes how everyone going splits into teams (first team = Team A). The first proposal opens a voting round; later ones join it. A valid proposal puts everyone going on exactly one team, nobody who isn't going, every team at or above min_players_per_team (sessions with teams), and uses the round's team count (the session's teams, else the first proposal's). Proposing never changes the current teams. Team sports only; not while a captain draft runs, and not while the current teams came from a completed captain draft (teams_source "draft": picked teams are not put to a vote) - reset the teams first (DELETE /sessions/{id}/teams), or replace them by hand.
//	@Tags			Team voting
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.ProposeTeamsRequest			true	"Teams in order"
//	@Success		201		{object}	api.Response[dtos.VotingResponse]	"Proposal added; the voting state"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"Session not found"
//	@Failure		409		{object}	api.Response[any]					"Caller not going (attendee_not_going), not enough people (not_enough_players), a draft is running (draft_already_active), the current teams came from a draft (teams_drafted), or session canceled/completed"
//	@Failure		422		{object}	api.Response[any]					"invalid_division (details.reason: wrong_team_count, player_not_going, duplicate_player, missing_players, team_below_minimum; details.user_ids), or not a team sport"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals [post]
func (h *Handler) ProposeTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.ProposeTeamsRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.Propose(ctx, *mapper.ProposeTeamsRequestToParams(&req, sessionID, accountID))
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteCreatedResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "Proposal added")
}

// VoteTeams handles PUT /api/v1/sessions/{id}/proposals/vote
//
//	@Summary		Vote for a proposal
//	@Description	Anyone going votes for one proposal, or to keep the current teams (only when the session had teams when voting opened). One vote per person; call again to change it.
//	@Tags			Team voting
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.CastVoteRequest				true	"proposal_id, or keep_current: true"
//	@Success		200		{object}	api.Response[dtos.VotingResponse]	"Vote recorded; the voting state"
//	@Failure		400		{object}	api.Response[any]					"Invalid request (give exactly one of proposal_id / keep_current)"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"Session or proposal not found"
//	@Failure		409		{object}	api.Response[any]					"Voting not open (voting_closed), or caller not going (attendee_not_going)"
//	@Failure		422		{object}	api.Response[any]					"keep_current_not_available"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals/vote [put]
func (h *Handler) VoteTeams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.CastVoteRequest
	if err := api.DecodeJSONBody(r, &req); err != nil {
		h.logger.Error(ctx, "failed to decode request body", "error", err)
		api.WriteValidationErrorResponse(w, err)
		return
	}

	if (req.ProposalID == nil) == !req.KeepCurrent {
		api.WriteBadRequestResponse(w, "invalid_vote", "Give exactly one of proposal_id or keep_current")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.Vote(ctx, application.CastVoteParams{
		SessionID:   sessionID,
		VoterID:     accountID,
		ProposalID:  req.ProposalID,
		KeepCurrent: req.KeepCurrent,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "")
}

// RetractVote handles DELETE /api/v1/sessions/{id}/proposals/vote
//
//	@Summary		Take your vote back
//	@Description	Removes the caller's own vote while voting is open, so they are "not voted" again. Idempotent: with no vote it changes nothing and still returns the voting state. Tallies and votes_cast drop, and the stream gets a voting.vote_retracted update.
//	@Tags			Team voting
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string								true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.VotingResponse]	"Vote removed; the voting state (my_vote null)"
//	@Failure		400	{object}	api.Response[any]					"Invalid request"
//	@Failure		401	{object}	api.Response[any]					"Unauthorized"
//	@Failure		404	{object}	api.Response[any]					"Session not found"
//	@Failure		409	{object}	api.Response[any]					"Voting not open (voting_closed), or caller not going (attendee_not_going)"
//	@Failure		500	{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals/vote [delete]
func (h *Handler) RetractVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.RetractVote(ctx, application.RetractVoteParams{
		SessionID: sessionID,
		VoterID:   accountID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "")
}

// WithdrawProposal handles DELETE /api/v1/sessions/{id}/proposals/{proposalId}
//
//	@Summary		Withdraw your own proposal
//	@Description	The author removes their proposal while voting is open. The votes for it are dropped (those voters are "not voted" again). If it was the last proposal the round is cancelled (cancel reason no_proposals) rather than left empty. The stream gets a voting.proposal_withdrawn update (with dropped_voter_ids), then voting.cancelled when the round ended.
//	@Tags			Team voting
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id			path		string								true	"Session ID"
//	@Param			proposalId	path		string								true	"Proposal ID"
//	@Success		200			{object}	api.Response[dtos.VotingResponse]	"Proposal withdrawn; the voting state"
//	@Failure		400			{object}	api.Response[any]					"Invalid request"
//	@Failure		401			{object}	api.Response[any]					"Unauthorized"
//	@Failure		403			{object}	api.Response[any]					"Not the proposal's author (not_proposal_author)"
//	@Failure		404			{object}	api.Response[any]					"Session or proposal not found (proposal_not_found)"
//	@Failure		409			{object}	api.Response[any]					"Voting not open (voting_closed)"
//	@Failure		500			{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals/{proposalId} [delete]
func (h *Handler) WithdrawProposal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	proposalID, err := uuid.Parse(r.PathValue("proposalId"))
	if err != nil {
		h.logger.Error(ctx, "invalid proposal ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_proposal_id", "Invalid proposal ID")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.WithdrawProposal(ctx, application.WithdrawProposalParams{
		SessionID:   sessionID,
		ProposalID:  proposalID,
		RequesterID: accountID,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "")
}

// CloseTeamVoting handles POST /api/v1/sessions/{id}/proposals/close
//
//	@Summary		Close team voting
//	@Description	The organizer closes voting. The option with the most votes wins: a proposal replaces the current teams (people who joined during the vote stay unassigned), "keep current teams" changes nothing. On a tie it fails with 409 tie_requires_winner and details.tied_proposal_ids / details.keep_current_tied; call again with winner_proposal_id or keep_current. Proposals and votes are deleted afterwards. Session creator or group admin/creator only.
//	@Tags			Team voting
//	@Accept			json
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		dtos.CloseVotingRequest				false	"Winner, only needed on a tie"
//	@Success		200		{object}	api.Response[dtos.VotingResponse]	"Voting closed; the voting state with result"
//	@Failure		400		{object}	api.Response[any]					"Invalid request"
//	@Failure		401		{object}	api.Response[any]					"Unauthorized"
//	@Failure		404		{object}	api.Response[any]					"Session not found"
//	@Failure		409		{object}	api.Response[any]					"tie_requires_winner, voting_closed, or session canceled/completed"
//	@Failure		422		{object}	api.Response[any]					"invalid_winner: not one of the options with the most votes"
//	@Failure		500		{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals/close [post]
func (h *Handler) CloseTeamVoting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	var req dtos.CloseVotingRequest
	if r.ContentLength != 0 {
		if err := api.DecodeJSONBody(r, &req); err != nil {
			h.logger.Error(ctx, "failed to decode request body", "error", err)
			api.WriteValidationErrorResponse(w, err)
			return
		}
	}

	if req.WinnerProposalID != nil && req.KeepCurrent {
		api.WriteBadRequestResponse(w, "invalid_winner", "Give at most one of winner_proposal_id or keep_current")
		return
	}

	session, err := h.sessionService.GetSession(ctx, sessionID, accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	requesterRole, err := h.getUserRole(ctx, session.ActivityGroupID(), accountID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.Close(ctx, application.CloseVotingParams{
		SessionID:         sessionID,
		RequesterID:       accountID,
		RequesterRole:     requesterRole,
		WinnerProposalID:  req.WinnerProposalID,
		WinnerKeepCurrent: req.KeepCurrent,
	})
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	h.logger.Info(ctx, "team voting closed", "session_id", sessionID)

	api.WriteOkResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "Voting closed")
}

// GetTeamVoting handles GET /api/v1/sessions/{id}/proposals
//
//	@Summary		Get team voting
//	@Description	The session's latest voting round - open (proposals with vote counts, the caller's vote, head count), closed (result) or cancelled (reason) - or voting_status "none". Proposals and votes only exist while voting is open.
//	@Tags			Team voting
//	@Produce		json
//	@Security		GatewayKeyAuth && BearerAuth
//	@Param			id	path		string								true	"Session ID"
//	@Success		200	{object}	api.Response[dtos.VotingResponse]	"Voting state"
//	@Failure		400	{object}	api.Response[any]					"Invalid request"
//	@Failure		401	{object}	api.Response[any]					"Unauthorized"
//	@Failure		404	{object}	api.Response[any]					"Session not found"
//	@Failure		500	{object}	api.Response[any]					"Internal server error"
//	@Router			/api/v1/sessions/{id}/proposals [get]
func (h *Handler) GetTeamVoting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	accountID := authcontext.GetAccountID(ctx)

	sessionID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		h.logger.Error(ctx, "invalid session ID", "error", err)
		api.WriteBadRequestResponse(w, "invalid_session_id", "Invalid session ID")
		return
	}

	if _, err := h.sessionService.GetSession(ctx, sessionID, accountID); err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	state, err := h.teamVotingService.GetVoting(ctx, sessionID)
	if err != nil {
		h.handleServiceError(ctx, w, err)
		return
	}

	api.WriteOkResponse(w, mapper.TeamVotingStateToResponse(state, accountID), "")
}
