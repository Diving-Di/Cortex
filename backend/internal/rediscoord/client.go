package rediscoord

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	address, username, password string
	database                    int
}

func New(raw string) (*Client, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "redis" || u.Host == "" {
		return nil, errors.New("invalid REDIS_URL")
	}
	db := 0
	if strings.Trim(u.Path, "/") != "" {
		db, err = strconv.Atoi(strings.Trim(u.Path, "/"))
		if err != nil {
			return nil, err
		}
	}
	password := ""
	username := ""
	if u.User != nil {
		username = u.User.Username()
		password, _ = u.User.Password()
	}
	return &Client{address: u.Host, username: username, password: password, database: db}, nil
}
func encode(args ...string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, x := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(x), x)
	}
	return []byte(b.String())
}
func readReply(r *bufio.Reader) (int64, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return 0, err
	}
	if len(line) < 3 {
		return 0, io.ErrUnexpectedEOF
	}
	switch line[0] {
	case ':':
		return strconv.ParseInt(strings.TrimSpace(line[1:]), 10, 64)
	case '+':
		return 1, nil
	case '-':
		return 0, errors.New(strings.TrimSpace(line[1:]))
	case '$':
		n, e := strconv.Atoi(strings.TrimSpace(line[1:]))
		if e != nil || n < 0 {
			return 0, e
		}
		buf := make([]byte, n+2)
		_, e = io.ReadFull(r, buf)
		return 1, e
	default:
		return 0, errors.New("unsupported Redis reply")
	}
}
func (c *Client) commands(ctx context.Context, commands ...[]string) ([]int64, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", c.address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	all := commands
	if c.password != "" {
		auth := []string{"AUTH", c.password}
		if c.username != "" {
			auth = []string{"AUTH", c.username, c.password}
		}
		all = append([][]string{auth}, all...)
	}
	if c.database != 0 {
		all = append([][]string{{"SELECT", strconv.Itoa(c.database)}}, all...)
	}
	for _, cmd := range all {
		if _, err = conn.Write(encode(cmd...)); err != nil {
			return nil, err
		}
	}
	reader := bufio.NewReader(conn)
	results := make([]int64, 0, len(all))
	for range all {
		x, e := readReply(reader)
		if e != nil {
			return nil, e
		}
		results = append(results, x)
	}
	return results, nil
}
func (c *Client) Reserve(ctx context.Context, stockKey, claimedKey, member string, total int, ttl time.Duration) (int, error) {
	seconds := int64(ttl.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	script := `if redis.call('SISMEMBER',KEYS[2],ARGV[1])==1 then return 2 end local n=tonumber(redis.call('GET',KEYS[1]) or '0') if n<=0 then return 0 end redis.call('DECR',KEYS[1]) redis.call('SADD',KEYS[2],ARGV[1]) redis.call('EXPIRE',KEYS[2],ARGV[2]) return 1`
	results, err := c.commands(ctx, []string{"SET", stockKey, strconv.Itoa(total), "NX", "EX", strconv.FormatInt(seconds, 10)}, []string{"EVAL", script, "2", stockKey, claimedKey, member, strconv.FormatInt(seconds, 10)})
	if err != nil {
		return 0, err
	}
	return int(results[len(results)-1]), nil
}

func (c *Client) ReservePrepared(ctx context.Context, stockKey, claimedKey, windowKey, eligibleKey, pointsKey, pendingKey, member string) (int, error) {
	script := `if redis.call('EXISTS',KEYS[1])==0 or redis.call('EXISTS',KEYS[3])==0 then return -1 end local w=redis.call('HMGET',KEYS[3],'opens','closes') local now=tonumber(redis.call('TIME')[1]) if now<tonumber(w[1]) then return -2 end if now>=tonumber(w[2]) then return -3 end if redis.call('SISMEMBER',KEYS[2],ARGV[1])==1 then return 2 end if redis.call('SISMEMBER',KEYS[4],ARGV[1])==0 then return -4 end local n=tonumber(redis.call('GET',KEYS[1]) or '0') if n<=0 then return 0 end redis.call('DECR',KEYS[1]) redis.call('SADD',KEYS[2],ARGV[1]) redis.call('ZADD',KEYS[6],now,ARGV[1]) return 1`
	result, err := c.commands(ctx, []string{"EVAL", script, "6", stockKey, claimedKey, windowKey, eligibleKey, pointsKey, pendingKey, member})
	if err != nil {
		return 0, err
	}
	return int(result[len(result)-1]), nil
}

func (c *Client) WarmEvent(ctx context.Context, stockKey, claimedKey, windowKey, eligibleKey, pointsKey, pendingKey string, opensAt, closesAt time.Time, remaining int, cost int64, members []string, eligible map[string]int64, ttl time.Duration) error {
	seconds := max(1, int(ttl.Seconds()))
	args := []string{strconv.Itoa(seconds), strconv.Itoa(remaining), strconv.FormatInt(opensAt.Unix(), 10), strconv.FormatInt(closesAt.Unix(), 10), strconv.FormatInt(cost, 10), strconv.Itoa(len(members))}
	args = append(args, members...)
	for member, points := range eligible {
		args = append(args, member, strconv.FormatInt(points, 10))
	}
	script := `local ttl=ARGV[1] local now=tonumber(redis.call('TIME')[1]) redis.call('ZREMRANGEBYSCORE',KEYS[6],'-inf',now-30) redis.call('EXPIRE',KEYS[6],ttl) redis.call('HSET',KEYS[3],'opens',ARGV[3],'closes',ARGV[4],'cost',ARGV[5]) redis.call('EXPIRE',KEYS[3],ttl) if redis.call('ZCARD',KEYS[6])>0 then return 0 end redis.call('SET',KEYS[1],ARGV[2],'EX',ttl) redis.call('DEL',KEYS[2],KEYS[4],KEYS[5]) local claimedCount=tonumber(ARGV[6]) for i=1,claimedCount do redis.call('SADD',KEYS[2],ARGV[6+i]) end redis.call('EXPIRE',KEYS[2],ttl) local start=7+claimedCount for i=start,#ARGV,2 do redis.call('SADD',KEYS[4],ARGV[i]) redis.call('HSET',KEYS[5],ARGV[i],ARGV[i+1]) end redis.call('EXPIRE',KEYS[4],ttl) redis.call('EXPIRE',KEYS[5],ttl) return 1`
	cmd := []string{"EVAL", script, "6", stockKey, claimedKey, windowKey, eligibleKey, pointsKey, pendingKey}
	cmd = append(cmd, args...)
	_, err := c.commands(ctx, cmd)
	return err
}
func (c *Client) ConfirmReservation(ctx context.Context, pendingKey, member string) error {
	_, err := c.commands(ctx, []string{"ZREM", pendingKey, member})
	return err
}

func (c *Client) Compensate(ctx context.Context, stockKey, claimedKey, windowKey, pointsKey, pendingKey, member string) error {
	script := `redis.call('ZREM',KEYS[5],ARGV[1]) if redis.call('SREM',KEYS[2],ARGV[1])==1 then redis.call('INCR',KEYS[1]) end return 1`
	_, err := c.commands(ctx, []string{"EVAL", script, "5", stockKey, claimedKey, windowKey, pointsKey, pendingKey, member})
	return err
}

// ApplyTemplateEvent updates only rebuildable marketplace projections. PostgreSQL
// remains authoritative, so a Redis failure can be retried safely from outbox_events.
func (c *Client) ApplyTemplateEvent(ctx context.Context, eventID, publicID, eventType, visitor string, delta int64, at time.Time) error {
	zone, _ := time.LoadLocation("Asia/Shanghai")
	day := at.In(zone).Format("20060102")
	script := `if not redis.call('SET',KEYS[1],'1','NX','EX',ARGV[1]) then return 0 end local t=ARGV[2] local d=tonumber(ARGV[3]) if t=='template.published' then redis.call('ZADD',KEYS[2],ARGV[4],ARGV[5]) elseif t=='template.like' then redis.call('ZINCRBY',KEYS[3],3*d,ARGV[5]) elseif t=='template.favorite' then redis.call('ZINCRBY',KEYS[3],5*d,ARGV[5]) elseif t=='template.used' then redis.call('ZINCRBY',KEYS[3],8*d,ARGV[5]) elseif t=='template.viewed' then redis.call('ZINCRBY',KEYS[3],d,ARGV[5]) redis.call('ZINCRBY',KEYS[4],d,ARGV[5]) if ARGV[6]~='' then redis.call('PFADD',KEYS[5],ARGV[6]) end end return 1`
	_, err := c.commands(ctx, []string{"EVAL", script, "5", "diary:outbox:processed:" + eventID, "diary:tpl:rank:new", "diary:tpl:rank:trending", "diary:tpl:rank:daily:" + day, "diary:tpl:uv:" + publicID + ":" + day, "691200", eventType, strconv.FormatInt(delta, 10), strconv.FormatInt(at.Unix(), 10), publicID, visitor})
	return err
}

func (c *Client) ApplyTemplateProjection(ctx context.Context, eventID, publicID, eventType, visitor string, publishedAt time.Time, trending, daily float64) error {
	zone, _ := time.LoadLocation("Asia/Shanghai")
	day := time.Now().In(zone).Format("20060102")
	script := `if not redis.call('SET',KEYS[1],'1','NX','EX',ARGV[1]) then return 0 end redis.call('ZADD',KEYS[2],ARGV[2],ARGV[5]) redis.call('ZADD',KEYS[3],ARGV[3],ARGV[5]) redis.call('ZADD',KEYS[4],ARGV[4],ARGV[5]) if ARGV[6]=='template.viewed' and ARGV[7]~='' then redis.call('PFADD',KEYS[5],ARGV[7]) end return 1`
	_, err := c.commands(ctx, []string{"EVAL", script, "5", "diary:outbox:processed:" + eventID, "diary:tpl:rank:new", "diary:tpl:rank:trending", "diary:tpl:rank:daily:" + day, "diary:tpl:uv:" + publicID + ":" + day, "691200", strconv.FormatInt(publishedAt.Unix(), 10), strconv.FormatFloat(trending, 'f', -1, 64), strconv.FormatFloat(daily, 'f', -1, 64), publicID, eventType, visitor})
	return err
}

func (c *Client) DeleteTemplateProjections(ctx context.Context, publicID string) error {
	_, err := c.commands(ctx,
		[]string{"ZREM", "diary:tpl:rank:new", publicID},
		[]string{"ZREM", "diary:tpl:rank:trending", publicID},
		[]string{"DEL", "diary:tpl:detail:" + publicID})
	return err
}

func (c *Client) Once(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	seconds := max(1, int(ttl.Seconds()))
	results, err := c.commands(ctx, []string{"SET", key, "1", "NX", "EX", strconv.Itoa(seconds)})
	if err != nil {
		return false, err
	}
	return results[len(results)-1] == 1, nil
}

func (c *Client) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	seconds := max(1, int(window.Seconds()))
	script := `local n=redis.call('INCR',KEYS[1]) if n==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]) end if n>tonumber(ARGV[2]) then return 0 end return 1`
	result, err := c.commands(ctx, []string{"EVAL", script, "1", key, strconv.Itoa(seconds), strconv.Itoa(limit)})
	if err != nil {
		return false, err
	}
	return result[len(result)-1] == 1, nil
}

func (c *Client) Get(ctx context.Context, key string) (string, bool, error) {
	return c.stringCommand(ctx, "GET", key)
}

func (c *Client) Score(ctx context.Context, key, member string) (float64, bool, error) {
	value, ok, err := c.stringCommand(ctx, "ZSCORE", key, member)
	if err != nil || !ok {
		return 0, ok, err
	}
	score, err := strconv.ParseFloat(value, 64)
	return score, true, err
}

func (c *Client) stringCommand(ctx context.Context, args ...string) (string, bool, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", c.address)
	if err != nil {
		return "", false, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	auth := []string{"AUTH", c.password}
	if c.username != "" {
		auth = []string{"AUTH", c.username, c.password}
	}
	for _, cmd := range [][]string{auth, {"SELECT", strconv.Itoa(c.database)}} {
		if (cmd[0] == "AUTH" && c.password == "") || (cmd[0] == "SELECT" && c.database == 0) {
			continue
		}
		if _, err = conn.Write(encode(cmd...)); err != nil {
			return "", false, err
		}
		if _, err = readReply(reader); err != nil {
			return "", false, err
		}
	}
	if _, err = conn.Write(encode(args...)); err != nil {
		return "", false, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", false, err
	}
	if len(line) < 3 || line[0] != '$' {
		return "", false, errors.New("unexpected Redis GET reply")
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return "", false, err
	}
	if n < 0 {
		return "", false, nil
	}
	buf := make([]byte, n+2)
	if _, err = io.ReadFull(reader, buf); err != nil {
		return "", false, err
	}
	return string(buf[:n]), true, nil
}

func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	seconds := max(1, int(ttl.Seconds()))
	_, err := c.commands(ctx, []string{"SET", key, value, "EX", strconv.Itoa(seconds)})
	return err
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	cmd := append([]string{"DEL"}, keys...)
	_, err := c.commands(ctx, cmd)
	return err
}

type RankingProjection struct {
	PublicID      string
	PublishedAt   time.Time
	TrendingScore float64
}

type RankedItem struct {
	Member string
	Score  float64
}

func (c *Client) RankingPage(ctx context.Context, key string, maxScore *float64, limit int) ([]RankedItem, error) {
	maxValue := "+inf"
	if maxScore != nil {
		maxValue = strconv.FormatFloat(*maxScore, 'f', -1, 64)
	}
	values, err := c.arrayCommand(ctx, "ZRANGE", key, maxValue, "-inf", "BYSCORE", "REV", "WITHSCORES", "LIMIT", "0", strconv.Itoa(max(1, limit*4)))
	if err != nil {
		return nil, err
	}
	result := make([]RankedItem, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		score, e := strconv.ParseFloat(values[i+1], 64)
		if e != nil {
			return nil, e
		}
		result = append(result, RankedItem{Member: values[i], Score: score})
	}
	return result, nil
}

func (c *Client) arrayCommand(ctx context.Context, args ...string) ([]string, error) {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", c.address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	auth := []string{"AUTH", c.password}
	if c.username != "" {
		auth = []string{"AUTH", c.username, c.password}
	}
	for _, cmd := range [][]string{auth, {"SELECT", strconv.Itoa(c.database)}} {
		if (cmd[0] == "AUTH" && c.password == "") || (cmd[0] == "SELECT" && c.database == 0) {
			continue
		}
		if _, err = conn.Write(encode(cmd...)); err != nil {
			return nil, err
		}
		if _, err = readReply(reader); err != nil {
			return nil, err
		}
	}
	if _, err = conn.Write(encode(args...)); err != nil {
		return nil, err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 3 || line[0] != '*' {
		return nil, errors.New("unexpected Redis array reply")
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[1:]))
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, n)
	for i := 0; i < n; i++ {
		header, e := reader.ReadString('\n')
		if e != nil {
			return nil, e
		}
		if len(header) < 3 || header[0] != '$' {
			return nil, errors.New("unexpected Redis bulk reply")
		}
		size, e := strconv.Atoi(strings.TrimSpace(header[1:]))
		if e != nil || size < 0 {
			return nil, e
		}
		buf := make([]byte, size+2)
		if _, e = io.ReadFull(reader, buf); e != nil {
			return nil, e
		}
		values = append(values, string(buf[:size]))
	}
	return values, nil
}

func (c *Client) RebuildTemplateRankings(ctx context.Context, items []RankingProjection) error {
	if _, err := c.commands(ctx, []string{"DEL", "diary:tpl:rank:new", "diary:tpl:rank:trending"}); err != nil {
		return err
	}
	for start := 0; start < len(items); start += 100 {
		end := min(len(items), start+100)
		commands := make([][]string, 0, (end-start)*2)
		for _, item := range items[start:end] {
			commands = append(commands, []string{"ZADD", "diary:tpl:rank:new", strconv.FormatInt(item.PublishedAt.Unix(), 10), item.PublicID}, []string{"ZADD", "diary:tpl:rank:trending", strconv.FormatFloat(item.TrendingScore, 'f', -1, 64), item.PublicID})
		}
		if _, err := c.commands(ctx, commands...); err != nil {
			return err
		}
	}
	return nil
}
