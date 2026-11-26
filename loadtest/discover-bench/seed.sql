-- Seeds activity.session for the discover benchmark (run.sh). Variables:
--   n          upcoming public sessions: the rows the discover query and its partial index are about
--   requester  the account that asks (its blocks are seeded)
--
-- Around them go 4n other rows - completed and cancelled history and private sessions, which the partial
-- index leaves out but which make the table the size it would really be. Sessions are spread over Croatian
-- cities by weight (a capital with far more than the rest), each jittered about 17 km north-south and 22 km
-- east-west, so a 50 km search around Zagreb finds a share of them, like the real one would.

TRUNCATE activity.session CASCADE;
TRUNCATE activity.account_block;

WITH cities(name, lat, lng, weight) AS (VALUES
    ('Zagreb', 45.8150, 15.9819, 40), ('Split', 43.5081, 16.4402, 15), ('Rijeka', 45.3271, 14.4422, 10),
    ('Osijek', 45.5550, 18.6955, 7), ('Zadar', 44.1194, 15.2314, 5), ('Dubrovnik', 42.6507, 18.0944, 5),
    ('Pula', 44.8666, 13.8496, 4), ('Varazdin', 46.3057, 16.3366, 3), ('Sibenik', 43.7350, 15.8952, 3),
    ('Karlovac', 45.4929, 15.5553, 2), ('Sisak', 45.4660, 16.3731, 2), ('Slavonski Brod', 45.1600, 18.0156, 2),
    ('Bjelovar', 45.8986, 16.8423, 2)
), cumulative AS (
    SELECT name, lat, lng, sum(weight) OVER (ORDER BY name) AS upto FROM cities
), picks AS MATERIALIZED (
    SELECT g, random() * (SELECT max(upto) FROM cumulative) AS r FROM generate_series(1, :n * 5) AS g
)
INSERT INTO activity.session (
    created_by_id, title, activity_type, difficulty_level, location_lat, location_lng, location_city,
    location_country, start_time, end_time, capacity, status, visibility
)
SELECT
    md5((g % 5000)::text)::uuid,
    'Bench game ' || g,
    (ARRAY['running','basketball','football','tennis','volleyball','gym','cycling','hiking','other'])[1 + g % 9],
    (ARRAY['beginner','intermediate','advanced'])[1 + g % 3],
    round((c.lat + (random() - 0.5) * 0.30)::numeric, 6),
    round((c.lng + (random() - 0.5) * 0.40)::numeric, 6),
    c.name,
    'Croatia',
    s.start_time,
    CASE WHEN g % 2 = 0 THEN s.start_time + interval '2 hours' END,  -- the rest have no end: the 3 h default
    20,
    s.status,
    s.visibility
FROM picks
JOIN LATERAL (SELECT name, lat, lng FROM cumulative WHERE upto >= picks.r ORDER BY upto LIMIT 1) c ON true
CROSS JOIN LATERAL (
    SELECT
        CASE WHEN g <= :n OR g % 10 >= 8
             THEN now() + interval '2 hours' + random() * interval '30 days'   -- upcoming
             ELSE now() - interval '1 day' - random() * interval '365 days'    -- history
        END AS start_time,
        CASE WHEN g <= :n THEN 'scheduled'
             WHEN g % 10 >= 8 THEN 'scheduled'
             WHEN g % 10 = 7 THEN 'cancelled'
             ELSE 'completed'
        END AS status,
        CASE WHEN g <= :n THEN 'public' WHEN g % 10 >= 8 THEN 'private' ELSE 'public' END AS visibility
) s;

-- the requester has blocked ten accounts that create sessions
INSERT INTO activity.account_block (blocker_id, blocked_id)
SELECT :'requester'::uuid, md5(i::text)::uuid FROM generate_series(1, 10) AS i;

ANALYZE activity.session;
ANALYZE activity.account_block;

SELECT count(*) AS rows_total,
       count(*) FILTER (WHERE status = 'scheduled' AND visibility = 'public') AS upcoming_public
FROM activity.session;
