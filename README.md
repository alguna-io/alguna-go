# Alguna Go SDK

The official Go SDK for the [Alguna API](https://alguna.com/docs): usage-based billing,
subscriptions, invoicing and payments.

## Install

```sh
go get github.com/alguna-io/alguna-go
```

The SDK needs Go 1.21 or later. Use it on a server only: the API key is a secret.

## Client

Create an API key in the dashboard under **Settings → Connections → Developers → API keys**.
A key looks like `<key_id>.<key_secret>`.

```go
package main

import (
	"net/http"
	"os"

	"github.com/alguna-io/alguna-go/client"
	"github.com/alguna-io/alguna-go/option"
)

func main() {
	// With no options, the client reads the key from ALGUNA_API_KEY.
	c := client.NewAlguna()

	sandbox := client.NewAlguna(
		option.WithAPIKey(os.Getenv("ALGUNA_SANDBOX_API_KEY")),
		option.WithAPIVersion("2026-04-01"),                       // default: the version this SDK release targets
		option.WithBaseURL("https://api.sandbox.alguna.io"),       // default: https://api.alguna.io
		option.WithHTTPHeader(http.Header{"X-Trace-Id": {"abc"}}), // sent on every request
	)
	_, _ = c, sandbox
}
```

`client.NewAlguna` cannot return an error, so it does not check the key. A client without a
key sends each request anyway, and the API answers with a `401`: the call returns an
`*alguna.UnauthorizedError` whose message says why. Pin the API version to keep your
integration on one version when you upgrade the module. The SDK sends
`User-Agent: alguna-go/<version>`; a `User-Agent` in `option.WithHTTPHeader` replaces it.

## Requests

The client has one field per API resource (`c.Customers`, `c.Subscriptions`, `c.Invoices`,
`c.Wallets`, …) and one method per operation. The request and response types are in the
`alguna` package. Every method takes a `context.Context` first.

```go
customer, err := c.Customers.Create(ctx, &alguna.CreateCustomerRequest{
	Name:     "Acme Corp",
	Currency: "USD",
})
if err != nil {
	return err
}

fetched, err := c.Customers.Get(ctx, customer.ID, option.WithAPIVersion("2026-04-01"))
if err != nil {
	return err
}
fmt.Println(fetched.Name)
```

Every method takes per-call options as its last arguments: `option.WithIdempotencyKey`
(writes), `option.WithMaxAttempts`, `option.WithoutRetries`, `option.WithHTTPClient`,
`option.WithHTTPHeader` and `option.WithAPIVersion`. To get the status code and headers as
well, call the same method on `WithRawResponse`, for example
`c.Customers.WithRawResponse.Get(ctx, id)`. `Invoices.GeneratePdf`, `Quotes.GetPdf` and
`Subscriptions.DownloadPdf` return an `io.Reader`.

## Pagination

A list method returns a `*core.Page`: `Results` holds the items of the current page, and
`Response` the full body. `Iterator` walks every item and gets the next page when it needs
one.

```go
page, err := c.Customers.List(ctx, &alguna.ListCustomersRequest{Limit: alguna.Int(100)})
if err != nil {
	return err
}
iter := page.Iterator()
for iter.Next(ctx) {
	fmt.Println(iter.Current().ID)
}
if err := iter.Err(); err != nil {
	return err
}
```

The API sends at most the operation's `limit` maximum per page. The next page starts after
the items that the SDK got, so a larger `limit` skips nothing. Iteration stops at the first
empty page, so a walk ends with one request that returns no items. To page by hand, call
`page.GetNextPage(ctx)`: after the last page it returns `core.ErrNoPages`.

## Idempotency and retries

Every request that is not a GET sends an `Idempotency-Key`: your key, or a UUID v4 that the
SDK makes for the call. Every retry of the call sends the same key, so the API returns the
first result and does not do the write again. Pass your own key when you repeat a call
yourself, for example after a restart:

```go
created, err := c.Customers.Create(ctx,
	&alguna.CreateCustomerRequest{Name: "Acme Corp", Currency: "USD"},
	option.WithIdempotencyKey(alguna.String("signup-1234")),
)
if err != nil {
	return err
}
fmt.Println(created.ID)
```

`Events.Ingest`, `Credits.Check` and `Credits.Track` are different. The API identifies their
work by the request body, so the SDK makes no key for them and still retries them. The API
removes duplicate events by `UniqueID`, and `Credits.Track` draws credit once for the same
`UniqueID`s. A key replaces that identity on `Credits.Track`, so a new key on each call would
make a redelivered batch draw credit twice. A key that you pass is sent on every attempt.
`Credits.Check` with `Reserve` needs your key, because the key identifies the hold.

A failed attempt is retried 2 times by default (3 attempts in total) when:

- the status is `408`, `429`, `502`, `503` or `504`;
- the status is `409` with a `Retry-After` header. The API sends this only while a request
  with the same key is still running. A `409` without `Retry-After` is a real conflict and is
  not retried;
- no response arrived: the connection was refused, reset or closed, or the attempt timed out.

The SDK does not retry after `ctx` is cancelled or its deadline passes. Between attempts it
waits 1 s, then 2 s, with random jitter of up to 10%. When the response has a `Retry-After`
header (seconds or an HTTP date), it waits that long instead, at most 60 s.
`option.WithMaxAttempts(n)` sets the number of attempts, the first included, on the client
or on one call. `option.WithMaxAttempts(0)` means the default of 3 attempts. To turn retries
off, use `option.WithoutRetries()`.

## Timeouts

Each attempt times out after 60 s (`core.DefaultTimeout`). A timed-out attempt is retried,
so one call can take 3 × the timeout plus the waits. To set a different time per attempt,
pass your own `http.Client` with its `Timeout`: it replaces the client that the SDK makes.
To limit the whole call, retries included, give `ctx` a deadline.

```go
c := client.NewAlguna(option.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}))

// Each attempt may take 10 s; the call, with its retries, at most 25 s.
ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
defer cancel()
customer, err := c.Customers.Get(ctx, "cus_123")
if err != nil {
	return err
}
fmt.Println(customer.Name)
```

## Errors

When the API answers with an error status, the call returns one of the error types in the
table below. Each type embeds `*core.APIError`, which has `StatusCode` and `Header`.
`err.Error()` is the status and the body's `detail`, else its
`title`, else its `message`, else the HTTP status text: for example,
`404: customer not found`. A failure with no response is the `*url.Error` from `net/http`.

```go
_, err := c.Customers.Get(ctx, "cus_missing")
var notFound *alguna.NotFoundError
var apiErr *core.APIError
switch {
case errors.As(err, &notFound):
	fmt.Println("no such customer:", notFound.Body.Detail)
case errors.As(err, &apiErr):
	fmt.Println(apiErr.StatusCode, apiErr.Header.Get("X-Request-Id"), err)
case err != nil:
	return err
}
```

| Error                              | Status                 |
| ---------------------------------- | ---------------------- |
| `*alguna.BadRequestError`          | 400                    |
| `*alguna.UnauthorizedError`        | 401                    |
| `*alguna.ForbiddenError`           | 403                    |
| `*alguna.NotFoundError`            | 404                    |
| `*alguna.ConflictError`            | 409                    |
| `*alguna.UnprocessableEntityError` | 422                    |
| `*alguna.TooManyRequestsError`     | 429                    |
| `*alguna.InternalServerError`      | 500                    |
| `*alguna.BadGatewayError`          | 502                    |
| `*alguna.ServiceUnavailableError`  | 503                    |
| `*alguna.GatewayTimeoutError`      | 504                    |
| `*core.APIError`                   | any other error status |

Each typed error has the decoded body in `Body`. When the body does not decode, as with the
empty body of some `502` responses, the error is a plain `*core.APIError`.

## Webhooks

The `alguna` package has one type for each webhook body, such as
`alguna.WebhookPaymentCreatedPayload`. Read `type` first, then decode the body into the
matching type:

```go
http.HandleFunc("/webhooks/alguna", func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read the body", http.StatusBadRequest)
		return
	}
	// Verify the Svix signature of body here, before you trust it.
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "the body is not JSON", http.StatusBadRequest)
		return
	}
	switch event.Type {
	case "payment.created":
		var payment alguna.WebhookPaymentCreatedPayload
		if err := json.Unmarshal(body, &payment); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Println(payment.Data.ID, payment.Data.Amount) // the amount is a decimal string
	default:
		// New event types can arrive before you upgrade.
	}
	w.WriteHeader(http.StatusNoContent)
})
```

The SDK types a payload; it does not check that Alguna sent it. Alguna signs every delivery
with Svix. Before you trust a payload, verify its `svix-id`, `svix-timestamp` and
`svix-signature` headers against the raw request body with the Svix library
(`github.com/svix/svix-webhooks/go`): `svix.NewWebhook(secret)`, then
`Verify(body, r.Header)`. See
[Verifying signatures](https://alguna.com/docs/api-reference/v2/webhooks#verifying-signatures).

## Reference

- [reference.md](reference.md): every method of this SDK
- [API reference](https://alguna.com/docs/api-reference/v2/overview)
- [Errors](https://alguna.com/docs/api-reference/v2/errors) and
  [idempotency](https://alguna.com/docs/api-reference/v2/idempotency)
- Support: support@alguna.io, with the `X-Request-Id` header of the failed response

## About this repo

Alguna generates this SDK from the Alguna API spec in a private repository. Open issues
here. Alguna ports code changes to the generator, so a pull request here is not merged as is.
