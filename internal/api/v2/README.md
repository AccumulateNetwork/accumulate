# API

## Execute Dispatch

Execute TX requests can be handled locally, if the API has a local node and if a
given request routes to that node. Otherwise, requests must be dispatched to the
appropriate node. This is done at the API level, by dispatching JSON-RPC
requests.

To avoid excessive network traffic, execute requests are not dispatched
immediately. If no current dispatch routine is running, the execute routine will
spawn a new dispatch routine, queue up requests for a specified length of time
or a specified number of requests, and then dispatch all the requests in one
batch for each remote API. For example, if the queue duration is 1 second and
the queue depth is 100, requests will be dispatched 1 second after the request
that started the queue, or after the queue reaches 100 requests, which ever
comes first.