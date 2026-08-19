\set ON_ERROR_STOP on

BEGIN;

INSERT INTO users(username,email,password_hash)
SELECT :'prefix'||i, :'prefix'||i||'@example.invalid', 'loadtest-token-only'
FROM generate_series(0, :total_users::int-1) AS i;

INSERT INTO tenants(id,user_id,name,note_quota)
SELECT gen_random_uuid(),u.id,'AI event HTTP load test',10
FROM users u
WHERE u.username LIKE :'prefix'||'%';

INSERT INTO auth_tokens(token_hash,user_id,expires_at)
SELECT encode(digest(convert_to('loadtest-token:'||:'run_id'||':'||substring(u.username from char_length(:'prefix')+1)||':'||:'secret','UTF8'),'sha256'),'hex'),
       u.id,now()+interval '6 hours'
FROM users u
WHERE u.username LIKE :'prefix'||'%';

INSERT INTO notes(tenant_id,created_by,updated_by,type,title,content,note_date,word_count,created_at,updated_at)
SELECT t.id,u.id,u.id,'normal','Load qualification '||day,
       'qualified load test content xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
       e.event_date-day,100,e.opens_at-interval '1 minute',e.opens_at-interval '1 minute'
FROM users u
JOIN tenants t ON t.user_id=u.id
CROSS JOIN generate_series(0, :required_days::int-1) AS day
CROSS JOIN LATERAL (SELECT event_date,opens_at FROM ai_flash_events ORDER BY event_date DESC LIMIT 1) e
WHERE u.username LIKE :'prefix'||'%'
  AND substring(u.username from char_length(:'prefix')+1)::int < :eligible_users::int;

COMMIT;

SELECT :'run_id' run_id, :'prefix' prefix, :total_users::int total_users,
       :eligible_users::int eligible_users, :total_users::int-:eligible_users::int ineligible_users;
