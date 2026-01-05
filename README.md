# GitHub Conditional HTTP Transport [![Go Reference](https://pkg.go.dev/badge/github.com/bored-engineer/github-conditional-http-transport.svg)](https://pkg.go.dev/github.com/bored-engineer/github-conditional-http-transport)
A Golang [http.RoundTripper](https://pkg.go.dev/net/http#RoundTripper) optimized for caching responses from GitHub's REST API via [conditional requests (ETag)](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api?apiVersion=2022-11-28#use-conditional-requests-if-appropriate).

## GitHub REST API Rate-Limits
While the [GitHub REST API](https://docs.github.com/en/rest/about-the-rest-api/about-the-rest-api?apiVersion=2022-11-28) is incredibly powerful, it enforces some strict [rate-limits](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28) on incoming requests. As of February 2025, the _[primary](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#about-primary-rate-limits)_ rate-limits for the REST API are:

* `60` requests per hour for unauthenticated requests [[details]](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#primary-rate-limit-for-unauthenticated-users)
* `5,000` requests per hour for authenticated requests [[details]](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#primary-rate-limit-for-authenticated-users)
    * This quota is shared across all [personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) of the authenticated user AND any GitHub/OAuth applications that have been authorized to make requests on behalf of the authenticated user.
* `5,000` requests per hour for [GitHub Apps](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/about-creating-github-apps) authenticated via an [installation access token](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app) [[details]](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#primary-rate-limit-for-github-app-installations)
    * For installations on a repository owned by a [GitHub Enterprise Cloud (GHEC)](https://docs.github.com/en/enterprise-cloud@latest/admin/overview/about-github-enterprise-cloud) organization, this limit is increased to `15,000` requests per hour.
    * For non-GHEC repositories, the rate-limit will scale up based on the number of users and repositories the GitHub App is installed on, with an upper limit of `12,500` requests per hour.
* `1,000` requests per hour per repository for [GitHub Actions](https://docs.github.com/en/actions) workloads [[details]](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#primary-rate-limit-for-github_token-in-github-actions)
    * This quota is shared across _all_ running GitHub Actions workflows in a given repository.
    * For requests to resources that belong to a [GitHub Enterprise Cloud (GHEC)](https://docs.github.com/en/enterprise-cloud@latest/admin/overview/about-github-enterprise-cloud) organization, this limit is increased to `15,000` requests per hour.

For [GitHub Enterprise Server (GHES)](https://docs.github.com/en/enterprise-server@3.15/admin/overview/about-github-enterprise-server) _[primary](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#about-primary-rate-limits)_ rate-limits are disabled by default but can be enabled by an administrator.

## Conditional Requests
Fortunately, GitHub has implemented a feature called [conditional requests](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api?apiVersion=2022-11-28#use-conditional-requests-if-appropriate) which allows callers to make requests to the REST API _without_ counting against their REST API rate-limit if the response has not changed since the last request via [ETag headers](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/ETag), ex:

```console
$ curl --request GET \
    --url https://api.github.com/users/bored-engineer \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    --include
HTTP/1.1 200 OK
...
ETag: "888348e1cff03510691fbf1eb221df5cf3c3c4651d7275d118372876e8cf9f5d"
X-RateLimit-Used: 1

$ curl --request GET \
    --url https://api.github.com/users/bored-engineer \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    --header 'If-None-Match: "888348e1cff03510691fbf1eb221df5cf3c3c4651d7275d118372876e8cf9f5d"' \
    --include
HTTP/1.1 304 Not Modified
...
X-RateLimit-Used: 0
```

While this requires each client to use/implement their own [RFC 7234](https://tools.ietf.org/html/rfc7234) compliant cache for HTTP responses (ex: [bored-engineer/httpcache](https://github.com/bored-engineer/httpcache)), it can be an excellent option for applications that request REST API resources (ex: [/users/{username}](https://docs.github.com/en/rest/users/users?apiVersion=2022-11-28#get-a-user) or [/repos/{owner}/{repo}](https://docs.github.com/en/rest/repos/repos?apiVersion=2022-11-28#get-a-repository)) which are unlikely to change frequently.

## Authenticated Conditional Requests are broken
Unfortunately, when you actually try to implement [conditional requests](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api?apiVersion=2022-11-28#use-conditional-requests-if-appropriate) inside your client, you'll quickly find that they do not work well when combined with [authentication](https://docs.github.com/en/rest/authentication/authenticating-to-the-rest-api?apiVersion=2022-11-28). 

This is because the response `ETag` value is based (in part) on the `Authorization` header included in the request, ex:
```console
$ curl --request GET \
    --url https://api.github.com/users/bored-engineer \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    --header "Authorization: Bearer ${GITHUB_TOKEN}" \
    --include
HTTP/1.1 200 OK
...
ETag: "993db4dbff350f7d8d5a92c3926fdab6311ff93963bc237343f07302c3ee3335"

$ curl --request GET \
    --url https://api.github.com/users/bored-engineer \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    --header "Authorization: Bearer ${OTHER_GITHUB_TOKEN_FOR_SAME_USER}" \
    --include
HTTP/1.1 200 OK
...
ETag: "e2bf720fe34544a7dc8c92b200010af26c2b2b09801cfc9b763bd34b6a57e8bb"
```

On the surface, this behavior makes a lot of sense, the response from the REST API is going to change based on which authenticated user is making the request, or even for the same user if the token used for the request has been granted different permissions. 

However, this means that whenever the `Bearer` token used in the request changes (such as when a new personal access token is generated), the entire cache will become invalid and the client will quickly hit the REST API rate-limits.

This is especially problematic when using a [GitHub App installation access token](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-an-installation-access-token-for-a-github-app) (one of the most common/useful authentication schemes in an enterprise context) because the access token is rotated once every hour, effectively guaranteeing there will be no benefit from using [conditional requests](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api?apiVersion=2022-11-28#use-conditional-requests-if-appropriate).

When fetching _public_ resources you can work-around this problem by authenticating using [Basic Authentication](https://developer.mozilla.org/en-US/docs/Web/HTTP/Authentication#basic_authentication_scheme), providing the GitHub/OAuth App's Client ID and Client Secret as the username and password respectively. However, this only works for _public_ resources and still suffers the same problem when the Client Secret is rotated.

## Reverse Engineering the GitHub ETag algorithm
Ideally, GitHub would fix this problem by instead using a constant value _derived_ from the `Authorization` header in the calculation of the `ETag` value, such as the GitHub App ID or the User ID associated with a PAT

Until that happens, I've spent some time reverse-engineering the GitHub `ETag` algorithm to work-around this problem client-side...

> [!WARNING]
> This section describes the implementation of the `ETag` header based on obversations/testing on [github.com](https://github.com/) as of February 2025. Critically, this implementation _is not documented_ by GitHub and could change without any notice, breaking/invalidating this package/section.

At it's core, the `ETag` is a [SHA-256](https://en.wikipedia.org/wiki/SHA-2) hash of the HTTP response body (before any compression), _prepended_ with the _value_ of the following headers (in this order, and only if present), separated by a `:`...
* `Accept`
* `Authorization`
* `Cookie`

For example, if we _remove_ any of the above headers from the request (`Accept` is added by default by `curl`), the `ETag` is just the SHA-256 of the response body:
```console
$ curl --request GET \
    --url https://api.github.com/users/bored-engineer \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    --header "Accept:" \
    --user-agent "Go-http-client/1.1" \
    --verbose |
    sha256sum /dev/stdin
HTTP/1.1 200 OK
...
< etag: W/"09dc54dc03e2e556eac3b0aeec31a70ffc04dc18a985144174d184815fe6ddea"
...
09dc54dc03e2e556eac3b0aeec31a70ffc04dc18a985144174d184815fe6ddea  /dev/stdin
```

And the more complex case where both `Accept` and `Authorization` are present:
```console
$ {
    printf "*/*"; # Accept:
    printf ":";
    printf "Bearer ${GITHUB_TOKEN}"; # Authorization:
    printf ":";
    curl --request GET \
        --url https://api.github.com/users/bored-engineer \
        --header "X-GitHub-Api-Version: 2022-11-28" \
        --header "Accept: */*" \
        --header "Authorization: Bearer ${GITHUB_TOKEN}" \
        --user-agent "Go-http-client/1.1" \
        --verbose
} | sha256sum /dev/stdin
HTTP/1.1 200 OK
...
< etag: "993db4dbff350f7d8d5a92c3926fdab6311ff93963bc237343f07302c3ee3335"
...
993db4dbff350f7d8d5a92c3926fdab6311ff93963bc237343f07302c3ee3335  /dev/stdin
```

In both of the examples, the `User-Agent` is set to `Go-http-client/1.1` because the GitHub REST API will pretty-print the response JSON if the `User-Agent` contains "curl", "Wget", "Safari" or "Firefox". However this happens _after_ the `ETag` has been calculated, corrupting the checksum/demos.

## Putting it all together
Using this reverse-engineered `ETag` algorithm, we can develop a [http.RoundTripper](https://pkg.go.dev/net/http#RoundTripper) that allows a GitHub REST API response that was cached/returned for a _different_ `Authorization` header to be safely reused. The logic for handling a HTTP request is roughly:
* If the HTTP request method is anything other than `GET` or `HEAD`
    * Return early, executing the request as-is because it will not be a [cacheable HTTP response](https://developer.mozilla.org/en-US/docs/Glossary/Cacheable)
* Retrieve the cached HTTP response body bytes from the cache storage using the URL as the key:
    * If no cached HTTP response is available, return early, executing the request as-is
* Calculate the _expected_ `ETag` (via SHA-256) using the request HTTP headers and the cached response body bytes
* Add the _expected_ `ETag` to the request via the `If-None-Modified` header, then perform the HTTP request
* If the HTTP response code is `304 Not Modified`, our cached value is still valid
    * Return the response headers (request-id, ratelimit headers, etc) but the cached HTTP response bytes
* If the HTTP response code is `200 OK` or `201 Created` and an `ETag` header is present
    * Store the response bytes in the cache storage
* Return the HTTP response

## Usage
Here is some example usage using the [bbolt](./bbolt) storage backend and the [google/go-github](https://github.com/google/go-github) client:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	ghtransport "github.com/bored-engineer/github-conditional-http-transport"
	bboltstorage "github.com/bored-engineer/github-conditional-http-transport/bbolt"
	"github.com/google/go-github/v68/github"
)

func main() {
	client := github.NewClient(&http.Client{
		Transport: ghtransport.NewTransport(
			bboltstorage.MustOpen("cache.db", 0644, nil, nil),
			http.DefaultTransport,
		),
	}).WithAuthToken(os.Getenv("GITHUB_TOKEN"))

	for loop := 0; loop < 3; loop++ {
		_, resp, err := client.Users.Get(context.TODO(), "bored-engineer")
		if err != nil {
			log.Fatalf("(*github.Client).Users.Get failed: %v", err)
		}
		log.Println(resp.Header.Get("X-Ratelimit-Used"))
	}
}
```
