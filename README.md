# zyzzyva

-primary: Primary replica ID (default: 0)
-rate: Requests per second (default: 0, unlimited)
-timeout: Request timeout (default: 10s)
-payload-size: Size of request payload in bytes (default: 64)
-requests: Number of requests to send (default: 0, unlimited)
-read-only: Whether requests are read-only (default: false)

Requires N = 3 * F + 1 servers and M clients, server or client is decided by whether id >= N

## License

MIT
