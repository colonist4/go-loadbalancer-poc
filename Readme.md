## Simple PoC of load balancer
Load balancer using token bucket algorithm


### Supported features
- Rate limit (Request body bytes per minute)
- Rate limit (Request per minute)
- Multiple backend support


### Run
1. Run backend servers first
```
go run main.go -backend -port 8081
go run main.go -backend -port 8082
```

2. Run load balancer
```
go run main.go
```

3. Send request to load balancer!
```
curl http://localhost:8080
```
