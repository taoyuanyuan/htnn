// Copyright The HTNN Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package limit_count_redis

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"mosn.io/htnn/pkg/expr"
	"mosn.io/htnn/pkg/filtermanager/api"
	"mosn.io/htnn/pkg/request"
	"mosn.io/htnn/pkg/stringx"
)

func configFactory(c interface{}) api.FilterFactory {
	conf := c.(*config)
	return func(callbacks api.FilterCallbackHandler) api.Filter {
		return &filter{
			callbacks: callbacks,
			config:    conf,
		}
	}
}

type filter struct {
	api.PassThroughFilter

	callbacks api.FilterCallbackHandler
	config    *config

	ress []interface{}
}

func (f *filter) getKey(script expr.Script, headers api.RequestHeaderMap) string {
	var key string
	if script != nil {
		res, err := script.EvalWithRequest(f.callbacks, headers)
		if err == nil {
			key = res.(string)
		}
	}
	if key == "" {
		api.LogInfo("limitCountRedis filter uses client IP as key because the configured key is empty")
		key = request.GetRemoteIP(f.callbacks.StreamInfo())
	}
	return key
}

var (
	redisScript = stringx.CutSpace(`
	local res={}
	for i=1,%d do
		local ttl=redis.call('ttl',KEYS[i])
		if ttl<0 then
			redis.call('set',KEYS[i],ARGV[i*2-1]-1,'EX',ARGV[i*2])
			res[i*2-1]=ARGV[i*2-1]-1
			res[i*2]=tonumber(ARGV[i*2])
		else
			res[i*2-1]=redis.call('incrby',KEYS[i],-1)
			res[i*2]=ttl
		end
	end
	return res
	`)
)

func (f *filter) DecodeHeaders(headers api.RequestHeaderMap, endStream bool) api.ResultAction {
	ctx := context.Background()
	config := f.config
	n := len(config.limiters)
	keys := make([]string, n)
	args := make([]interface{}, n*2)
	for i, limiter := range config.limiters {
		key := f.getKey(limiter.script, headers)
		keys[i] = limiter.prefix + "|" + key

		api.LogInfof("limitCountRedis filter, key: %s", key)

		args[i*2] = limiter.count
		args[i*2+1] = limiter.timeWindow
	}

	cmd := config.client.Eval(ctx, fmt.Sprintf(redisScript, n), keys, args...)
	res, err := cmd.Result()
	if err != nil {
		api.LogErrorf("failed to limit count: %v", err)

		if config.FailureModeDeny {
			return &api.LocalResponse{Code: 503}
		}
		return api.Continue
	}

	ress := res.([]interface{})
	f.ress = ress

	for i := range config.limiters {
		remain := ress[2*i].(int64)
		if remain < 0 {
			hdr := http.Header{}
			// TODO: add option to disable x-envoy-ratelimited
			hdr.Set("x-envoy-ratelimited", "true")
			return &api.LocalResponse{Code: 429, Header: hdr}
		}
	}

	return api.Continue
}

func (f *filter) EncodeHeaders(headers api.ResponseHeaderMap, endStream bool) api.ResultAction {
	config := f.config
	if !config.EnableLimitQuotaHeaders {
		return api.Continue
	}

	var minCount uint32
	var minRemain int64 = math.MaxUint32
	var minTTL int64
	for i, lim := range f.config.limiters {
		remain := f.ress[2*i].(int64)
		ttl := f.ress[2*i+1].(int64)

		if remain < minRemain {
			minRemain = remain
			minCount = lim.count
			minTTL = ttl
		}
	}

	// According to the RFC, these headers MUST NOT occur multiple times.
	headers.Add("x-ratelimit-limit", fmt.Sprintf("%d, %s", minCount, config.quotaPolicy))
	if minRemain <= 0 {
		headers.Add("x-ratelimit-remaining", "0")
	} else {
		headers.Add("x-ratelimit-remaining", strconv.FormatInt(minRemain, 10))
	}
	headers.Add("x-ratelimit-remaining", strconv.FormatInt(minRemain, 10))
	headers.Add("x-ratelimit-reset", strconv.FormatInt(minTTL, 10))
	return api.Continue
}