You are asked to implement an ip to country service. The service gets a REST API request with
an IP address and returns its location (country, city). The service must have a rate-limiting
mechanism and be easily extendable to support different ip2country databases.
Detailed requirements:
1. You must write clear and easy to read code.
2. Your service should have a mechanism to provide its configuration.
3. The service should receive a HTTP GET request to `/v1/find-country?ip=2.22.233.255` and
return a JSON response in the following format `{"country": "XXXX", "city": "XXXX"}`.
4. If an error is encountered you must return a JSON error response in the following format
`{"error": "XXX"}` with the appropriate HTTP status code.
5. The service should be easily extendable to support multiple ip2country databases, so design
your service with this in mind. The active datastore will be determined by an environment
variable. For the sake of the exercise you can use a text based comma separated format
(ip,city,country) or any other format that is easy for you to work with.
6. The service must have a request per second limiting mechanism. The rate limit should be
configured using an environment variable. For the sake of the exercise you must not use any
open source libraries that implement this (including the built-in `golang.org/x/time/rate`
package). When the rate limit is hit you must return 429 HTTP status code.
7. The service should be delivered in production-grade quality.

