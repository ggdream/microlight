package websearch

// These are product-specific codes and HTTP statuses whose meanings differ
// from the generic HTTP status. Response text also covers newly added codes.
var providerReasons = map[ID]map[string]Failure{
	Baidu: {
		"InternalError":                {Kind: "service_unavailable", Message: "Baidu Search reported an internal error.", Retryable: true},
		"InvalidArgument":              {Kind: "invalid_request", Message: "Baidu Search rejected a request parameter."},
		"RATE_LIMIT_SEARCH_QPS":        {Kind: "rate_limited", Message: "Baidu Search's per-user request rate limit was exceeded.", Retryable: true},
		"QUOTA_USER_DAILY_FREE":        {Kind: "daily_quota", Message: "Baidu Search's daily free quota was exhausted and pay-as-you-go is disabled."},
		"BILLING_INSUFFICIENT_BALANCE": {Kind: "billing", Message: "The Baidu account balance is insufficient."},
		"216003":                       {Kind: "authentication", Message: "Baidu rejected the API key authorization."},
	},
	Aliyun: {
		"InvalidAccessKeyId.NotFound":     {Kind: "authentication", Message: "Alibaba Cloud could not find the supplied access key."},
		"Retrieval.NotActivate":           {Kind: "service_not_enabled", Message: "Alibaba Cloud IQS has not been activated for this account."},
		"Retrieval.Arrears":               {Kind: "billing", Message: "The Alibaba Cloud account balance is insufficient."},
		"Retrieval.NotAuthorised":         {Kind: "access_denied", Message: "The Alibaba Cloud sub-account lacks permission to use IQS."},
		"Retrieval.TestUserPeriodExpired": {Kind: "trial_expired", Message: "The Alibaba Cloud IQS trial has expired."},
		"Retrieval.Throttling.User":       {Kind: "rate_limited", Message: "The Alibaba Cloud IQS request rate limit was exceeded.", Retryable: true},
		"Retrieval.TestUserQueryExceeded": {Kind: "daily_quota", Message: "The Alibaba Cloud IQS trial's daily search quota was exhausted."},
	},
	Tencent: {
		"InternalError":         {Kind: "service_unavailable", Message: "Tencent Yuanbao Search reported an internal error.", Retryable: true},
		"InvalidParameter":      {Kind: "invalid_request", Message: "Tencent Yuanbao Search rejected a request parameter."},
		"RequestLimitExceeded":  {Kind: "rate_limited", Message: "Tencent Yuanbao Search's request limit was exceeded.", Retryable: true},
		"ResourceNotFound":      {Kind: "service_not_enabled", Message: "Tencent Yuanbao Search has not been activated for this account."},
		"ResourceUnavailable":   {Kind: "service_unavailable", Message: "Tencent Yuanbao Search is unavailable for this account. Check the account's billing status."},
		"UnauthorizedOperation": {Kind: "authentication", Message: "The Tencent Yuanbao Search API key is invalid or unauthorized."},
	},
	Volcengine: {
		"10400":  {Kind: "invalid_request", Message: "Doubao Search rejected a request parameter."},
		"10401":  {Kind: "authentication", Message: "Doubao Search's gateway rejected the token."},
		"10402":  {Kind: "invalid_search_type", Message: "The requested Doubao search type is invalid or has not been enabled."},
		"10403":  {Kind: "access_denied", Message: "This account lacks permission to use the requested Doubao Search service."},
		"10406":  {Kind: "quota", Message: "Doubao Search's free quota was exhausted."},
		"10408":  {Kind: "service_not_enabled", Message: "Paid Doubao Search is not enabled for this account."},
		"10409":  {Kind: "unsupported_plan", Message: "The Doubao Search subscription does not cover this search type."},
		"10410":  {Kind: "subscription_unavailable", Message: "No active Doubao Search subscription is available for this API key."},
		"10412":  {Kind: "quota", Message: "The Doubao Search subscription quota was exhausted."},
		"10500":  {Kind: "service_unavailable", Message: "Doubao Search reported an internal error.", Retryable: true},
		"700429": {Kind: "rate_limited", Message: "Doubao Search's request rate limit was exceeded.", Retryable: true},
	},
	DeepSeek: {
		"402":                   {Kind: "billing", Message: "The DeepSeek account balance is insufficient."},
		"invalid_request_error": {Kind: "invalid_request", Message: "DeepSeek rejected the request headers or body."},
	},
	Tavily: {
		"432": {Kind: "quota", Message: "The Tavily plan's usage limit was exceeded."},
		"433": {Kind: "billing_limit", Message: "The Tavily pay-as-you-go spending limit was exceeded."},
		"434": {Kind: "service_unavailable", Message: "Live fetching is temporarily unavailable for this Tavily plan.", Retryable: true},
	},
	Firecrawl: {
		"402": {Kind: "billing", Message: "Firecrawl credits are exhausted or billing is not configured."},
	},
	Google: {
		"dailyLimitExceeded":      {Kind: "daily_quota", Message: "Google's daily search quota was exhausted."},
		"dailyLimitExceededUnreg": {Kind: "authentication", Message: "Google requires a valid project or API key to use this search service."},
		"keyInvalid":              {Kind: "authentication", Message: "Google rejected the API key."},
		"API_KEY_INVALID":         {Kind: "authentication", Message: "Google rejected the API key."},
		"accessNotConfigured":     {Kind: "service_not_enabled", Message: "Google Custom Search is not enabled for this project."},
		"SERVICE_DISABLED":        {Kind: "service_not_enabled", Message: "Google Custom Search is not enabled for this project."},
		"quotaExceeded":           {Kind: "quota", Message: "The Google search quota was exhausted."},
		"rateLimitExceeded":       {Kind: "rate_limited", Message: "The Google search request rate limit was exceeded.", Retryable: true},
		"userRateLimitExceeded":   {Kind: "rate_limited", Message: "The Google search per-user rate limit was exceeded.", Retryable: true},
	},
	SearXNG: {
		"403": {Kind: "access_denied", Message: "The SearXNG instance denied access. JSON output may be disabled."},
	},
	Wikipedia: {
		"maxlag":   {Kind: "service_unavailable", Message: "Wikipedia is throttling requests while its replicas catch up.", Retryable: true},
		"readonly": {Kind: "service_unavailable", Message: "Wikipedia is temporarily in read-only mode.", Retryable: true},
	},
}
