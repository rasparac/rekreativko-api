-- The discover query exactly as persistence/session_repo.go DiscoverSessions builds it for the first page
-- (no cursor, page size 20 + 1), with the requester's block filter (moderation_repo.go
-- sessionNotHiddenByBlock) and sessionEndExpr expanded. Variables: lat, lng, radius, requester, interests
-- (empty, or an 'AND (...)' clause). Literals instead of bind parameters, so the planner sees the values
-- (the service's driver may use a generic plan after a few calls: re-check there if the numbers matter).

SELECT :radius / 111.045 AS lat_delta, :radius / (111.045 * cos(radians(:lat))) AS lng_delta \gset

EXPLAIN (ANALYZE, BUFFERS)
SELECT * FROM (
    SELECT
        id, activity_group_id, created_by_id, session_template_id, location_city, location_country,
        location_lat, location_lng, start_time, end_time, capacity, status, visibility, is_recurring, note,
        created_at, updated_at, cancelled_at, started_at, completed_at, title, activity_type, difficulty_level,
        location_street, requires_approval, team_count, min_players_per_team, teams_source,
        (6371 * acos(LEAST(1, GREATEST(-1,
            cos(radians(:lat)) * cos(radians(location_lat)) *
            cos(radians(location_lng) - radians(:lng)) +
            sin(radians(:lat)) * sin(radians(location_lat))
        )))) AS distance_km
    FROM activity.session
    WHERE visibility = 'public'
        AND status = 'scheduled'
        AND deleted_at IS NULL
        AND COALESCE(end_time, start_time + interval '10800 seconds') > now()
        AND location_lat BETWEEN :lat - :lat_delta AND :lat + :lat_delta
        AND location_lng BETWEEN :lng - :lng_delta AND :lng + :lng_delta
        :interests
        AND (
            created_by_id NOT IN (SELECT blocked_id FROM activity.account_block WHERE blocker_id = :'requester'::uuid)
            OR id IN (SELECT session_id FROM activity.session_attendee WHERE account_id = :'requester'::uuid AND deleted_at IS NULL)
            OR activity_group_id IN (SELECT activity_group_id FROM activity.member WHERE account_id = :'requester'::uuid AND status = 'confirmed' AND deleted_at IS NULL)
        )
) nearby
WHERE distance_km <= :radius
    AND (NULL::float8 IS NULL OR (distance_km, id) > (NULL::float8, NULL::uuid))
ORDER BY distance_km ASC, id ASC
LIMIT 21;
