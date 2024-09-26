# Discussions for Lab 1
Name: Bill Qian

## A1

`NewClient` is similar to a combination of the `socket()` and `connect()` functions in the C-style socket API. It abstracts the process of creating a socket and establishing a connection, making it more user-friendly. Note that `socket()` creates a communication endpoint and `connect()` establishes a connection to a server, and `NewClient` just handles both tasks in a higher-level Go manner, with the option of additional configuration and error handling.

## A2

NewClient() might fail on the following conditions:
- The server address is invalid or the server is not running.
- The server is running but not accepting connections, or the connection is refused due to a firewall or other network configuration, or the connection queue is full.

In these cases, the best value to return would be codes.Unavailable, because it indicates that the server is indeed unavailable one reason or another. It the most simple solution to the problem. We could be more specific like:
```go
if err != nil {
    switch {
    case strings.Contains(err.Error(), "context deadline exceeded"):
        return nil, status.Errorf(codes.DeadlineExceeded, "VideoRecService: connection to UserService timed out: %v", err)
    case strings.Contains(err.Error(), "no such host"):
        return nil, status.Errorf(codes.InvalidArgument, "VideoRecService: invalid UserService address: %v", err)
    default:
        return nil, status.Errorf(codes.Unavailable, "VideoRecService: failed to connect to UserService: %v", err)
    }
}
```

but this approach is more complex and might make it too cumbersome to handle all the possible errors.

The other time where an error could occur is during closing the client connections. We also use codes.Unavailable in this case because the connections are "unavailable" to close. It is the most simple and straightforward code to use.


## A3

When `GetUser()` is called, it first serializes the request into a byte slice, then using the already established connection, sends the request to the server. This would call the `send()/write()` (note not `sendto()` because we have established a TCP connection (HTTP/2)). Then, we would use `select()/poll()/epoll()` to wait for the socket to become readable. Once the socket is readable, we would read the response from the server using `recv()/read()`. Then, we would deserialize the response and return it to the caller.

Now, let's list some potential error cases:
- Network-related: Connection closed, server not running, broken pipe, connection timeouts/resets (due to congestion)
  - Detected by OS when attempting to read/write socket
  - Detected by TCP timeout (as a connection reset/read/write error)
  - Detected through TCP congestion control algorithms
- Protocol-related errors: malformed request/response, version mismatch, etc
  - Detected during deserialization of Protobuf data (library will handle this)
  - gRPC includes version information in protocol. Mismatches are detected during initial handshake between client/server
- System errors: Out of file descriptors or memory
  - Detected when system calls like socket() fail with EMFILE
  - Detected when malloc fails
- Application-level errors: Server side database errors, timeouts, etc
  - Triggered by service-side events
  - Timeouts detected using timer mechanisms in gRPC library (returns a DEADLINE_EXCEEDED) error
- Client-level errors: Context canceled
  - gRPC checks context state before each operation, returns CANCELLED error if aborted

It's possible for GetUser to return errors when the network calls succeed, for example in the application-level errors (ex. user not found) or client-side ones. But the gRPC framework abstracts all of this away

The last layer of error of handling relies on the HTTP/2 protoco, which provides stream-level and connection-level error detection, flow control, etc.

## ExtraCredit1
(Same as above)
- Network-related: Connection closed, server not running, broken pipe, connection timeouts/resets (due to congestion)
    - Detected by OS when attempting to read/write socket
    - Detected by TCP timeout (as a connection reset/read/write error)
    - Detected through TCP congestion control algorithms
- Protocol-related errors: malformed request/response, version mismatch, etc
    - Detected during deserialization of Protobuf data (library will handle this)
    - gRPC includes version information in protocol. Mismatches are detected during initial handshake between client/server
- System errors: Out of file descriptors or memory
    - Detected when system calls like socket() fail with EMFILE
    - Detected when malloc fails
- Application-level errors: Server side database errors, timeouts, etc
    - Triggered by service-side events
    - Timeouts detected using timer mechanisms in gRPC library (returns a DEADLINE_EXCEEDED) error
- Client-level errors: Context canceled
    - gRPC checks context state before each operation, returns CANCELLED error if aborted

## A4

If we used the same connection for UserService and VideoService, first of all we would be assuming that UserService and VideoService as hosted at the same address, of which they are not. Second, we would be sending incorrectly serialized data (eg. UserService expects UserRequest over the connection, but it might recieve a VideoRequest) and not know what to do with it. 

On a separate note, would also break the gRPC method identification system, violate type safety guaranteed by generated stubs, and potentially cause security vulnerabilities if services have different authentication requirements. 

## A6
Here is my output:
```
nonroot@distributed:~/cs426-fall24/lab1$ go run cmd/frontend/frontend.go --net-id=bnq3
2024/09/24 17:22:26 Welcome bnq3! The UserId we picked for you is 203805.

2024/09/24 17:22:26 This user has name Bashirian8120, their email is audreybeahan@yost.com, and their profile URL is https://user-service.localhost/profile/203805
2024/09/24 17:22:26 Recommended videos:
2024/09/24 17:22:26   [0] Video id=1052, title="grieving Snow Peas", author=Jevon Botsford, url=https://video-data.localhost/blob/1052
2024/09/24 17:22:26   [1] Video id=1042, title="The purple yellowjacket's honesty", author=Terrence Schinner, url=https://video-data.localhost/blob/1042
2024/09/24 17:22:26   [2] Video id=1212, title="Foxride: unlock", author=Estella Emmerich, url=https://video-data.localhost/blob/1212
2024/09/24 17:22:26   [3] Video id=1334, title="GhostWhitescale: index", author=Heidi Shields, url=https://video-data.localhost/blob/1334
2024/09/24 17:22:26   [4] Video id=1181, title="dizzying behind", author=Lacy McDermott, url=https://video-data.localhost/blob/1181
2024/09/24 17:22:26 

==== BASIC TESTS ====
2024/09/24 17:22:26 Test case 1: UserId=204054
2024/09/24 17:22:27 Recommended videos:
2024/09/24 17:22:27   [0] Video id=1012, title="The eager ferret's punctuation", author=Rae Ziemann, url=https://video-data.localhost/blob/1012
2024/09/24 17:22:27   [1] Video id=1312, title="The lazy frog's unemployment", author=Harry Boehm, url=https://video-data.localhost/blob/1312
2024/09/24 17:22:27   [2] Video id=1209, title="Koalajump: compile", author=Amie Rau, url=https://video-data.localhost/blob/1209
2024/09/24 17:22:27   [3] Video id=1309, title="gleaming Jicama", author=Ana Wunsch, url=https://video-data.localhost/blob/1309
2024/09/24 17:22:27   [4] Video id=1079, title="precious here", author=George Morissette, url=https://video-data.localhost/blob/1079
2024/09/24 17:22:27 Test case 2: UserId=203584
2024/09/24 17:22:27 Recommended videos:
2024/09/24 17:22:27   [0] Video id=1196, title="The orange gnu's heat", author=Dagmar Hauck, url=https://video-data.localhost/blob/1196
2024/09/24 17:22:27   [1] Video id=1370, title="The hungry gerbil's safety", author=Rocky Okuneva, url=https://video-data.localhost/blob/1370
2024/09/24 17:22:27   [2] Video id=1071, title="The worrisome sheep's courage", author=Kayley Moore, url=https://video-data.localhost/blob/1071
2024/09/24 17:22:27   [3] Video id=1041, title="The kind hyena's unemployment", author=Layla Effertz, url=https://video-data.localhost/blob/1041
2024/09/24 17:22:27   [4] Video id=1240, title="witty downstairs", author=Toni Johnson, url=https://video-data.localhost/blob/1240
2024/09/24 17:22:27 OK: basic tests passed!
```

## A8
There are many different factors that tell us whether we should send batched requests concurrently, from server capacity, API design, use case requirements, and server-side implementation. For example, if server capacity is nearly infinite, concurrent batched requests might be beneficial (note that this is rarely the case in practice one is a small provider querying Google). Second, many APIs are designed with specific batch limits to ensure fair resource allocation and manage server load, and we should generally respect the limits. Third, servers may already handle concurrency internally for batched requests, making client-side concurrency unnecessary or even counterproductive as it might overload the server and defeat the purpose of even offering a batch API in the first place.

Of course, all of this depends on the nature of the operations being performed and the specific requirements of the application.

Some pros and cons of each are as follows:

Pros:

1. We get **improved performance** from sending batched requests concurrently. I.e. we significantly reduce overall response time, especially when dealing with independent operations.

2. Better **resource utilization** of available network and server resources, potentially improving throughput, and reducing the amount of waiting that has to happen per-thread.

3. Reduced **latency** for time-sensitive applications, concurrent requests can provide faster results by parallelizing operations (since we are not waiting for operations to happen sequentially)

Cons:
1.	We risk **server overload** from sending batched requests concurrently. I.e. if not properly managed, concurrent requests might overwhelm the server, potentially leading to degraded performance or failures.
2.	Increased **code complexity** for both client and server-side implementations. Implementing and managing concurrent requests adds complexity, which can lead to more difficult maintenance and debugging.
3.	Higher chance of **race conditions** for interdependent operations. Concurrent operations may lead to unexpected behavior if not carefully designed, especially when dealing with data that relies on a specific order of operations.
4.	Potential **violation of API design intent** if we bypass intended usage patterns. If the API was designed with specific batch limits to manage server load, using concurrent requests could be seen as circumventing these safeguards.

## ExtraCredit2

To minimize the total number of requests to VideoService and UserService given many requests, we can use a queueing system to ensure as many requests have BATCH_SIZE elements as possible. Here is an example of the components we would need:

1. Create a queue for each service (UserService and VideoService) to hold incoming requests, adding requests to the appropriate queue when they come in

2. Set up a background process that continuously monitors each queue, creating and sending batches of requests to the respective services.

3. The batching logic will work as follows:
  - The background process will check two conditions:
    a. If the current batch has reached the maximum size (BATCH_SIZE=50).
    b. If there are any requests in the batch and it's been waiting for longer than a set time limit (MAX_WAIT_TIME=probably 1 second).
  - If either is true, send the current batch, and then process the results and send them back to the original requesters.

4. While doing all of this, keep track of which requests in the batch correspond to which original function calls. This will allow us to return the correct results to the right callers. This operation runs continuously: if the queue isn't empty, keep moving requests from the queue into the current batch. If the queue is empty, wait for new requests to arrive or for the time limit to be reached.

6. When a new recommendation request comes in, we
    a. Add the user request to the UserService queue.
    b. Add the video request to the VideoService queue.
    c. Wait for both results to come back.
    d. Use these results to generate the recommendation.


Using this we significantly reduce the number of individual calls to UserService and VideoService. Instead of making a separate call for each user and video, we group these requests into batches, only sending requests when we have enough to fill a batch or when we've waited long enough. This results in fewer overall requests to these services, improving efficiency and reducing load on the system. However, note that this may increase latency for smaller request volumes, so we still have to balance between making efficient use of batching (by trying to fill batches) and maintaining responsiveness (by sending partially filled batches after a time limit).


## B2
My results with 10 qps is as follows (running on an M1 Max Macbook Pro under Ubuntu 22.04 with the Virtualization framework):

<details>
<summary>Latency Chart</summary>
<pre><code>
nonroot@distributed:~/cs426-fall24/lab1$ go run cmd/stats/stats.go 
now_us  total_requests  total_errors    active_requests user_service_errors     video_service_errors    average_latency_ms      p99_latency_ms  stale_responses
1727294340093346        0       0       0       0       0       0.00    0.00    0
1727294341095486        0       0       0       0       0       0.00    0.00    0
1727294342096533        0       0       0       0       0       0.00    0.00    0
1727294343103252        0       0       0       0       0       0.00    0.00    0
1727294344095682        13      0       1       0       0       144.17  214.00  0
1727294345094267        23      0       0       0       0       110.26  214.00  0
1727294346096385        33      0       1       0       0       97.25   214.00  0
1727294347094656        43      0       1       0       0       98.26   214.00  0
1727294348094527        53      0       1       0       0       105.13  213.86  0
1727294349096086        63      0       1       0       0       104.31  213.16  0
1727294350097902        73      0       1       0       0       106.56  212.46  0
1727294351094938        83      0       1       0       0       103.66  211.76  0
1727294352096365        93      0       1       0       0       106.43  211.06  0
1727294353096955        103     0       1       0       0       107.23  210.36  0
1727294354096237        113     0       1       0       0       108.48  209.66  0
1727294355094887        123     0       1       0       0       107.28  208.96  0
1727294356094384        133     0       1       0       0       108.92  208.26  0
1727294357096388        143     0       1       0       0       109.43  207.56  0
1727294358095334        153     0       1       0       0       109.76  206.34  0
1727294359094713        163     0       1       0       0       111.00  203.04  0
1727294360094422        173     0       0       0       0       110.47  199.41  0
1727294361094230        183     0       1       0       0       107.46  196.44  0
1727294362095180        193     0       0       0       0       106.72  192.81  0
</code></pre>
</details>

<details>
<summary>And after running 100 qps</summary>
<pre><code>
now_us  total_requests  total_errors    active_requests user_service_errors     video_service_errors    average_latency_ms      p99_latency_ms  stale_responses
...
1727294363100787        198     0       0       0       0       105.53  191.16  0
1727294364095389        198     0       0       0       0       105.53  191.16  0
1727294365095191        198     0       0       0       0       105.53  191.16  0
1727294366109653        324     0       113     0       0       112.09  270.99  0
1727294367209053        431     0       165     0       0       253.06  1300.36 0
1727294368118998        528     0       184     0       0       587.35  2262.18 0
1727294369153394        629     0       192     0       0       878.78  2545.95 0
1727294370206484        733     0       217     0       0       1038.67 2699.38 0
1727294371105813        831     0       240     0       0       1205.23 2802.36 0
1727294372343736        930     0       300     0       0       1295.68 2869.20 0
1727294373274811        1032    0       346     0       0       1451.32 3687.40 0
</code></pre>
</details>



## C1

Retrying can be a bad option because it can add unnecessary server load for requests known to fail. For example, the following errors probably shouldn't be retried:
-   4xx Client Errors: These indicate issues with the request itself. Common ones include:
- InvalidArgument: invalid request parameters.
- Unauthenticated: authentication required.
- PermissionDenied: permission denied.
- NotFound: resource not found.
-   5xx Server Errors that indicate issues that won't resolve with retries:
- Unimplemented: Method not supported.
- Internal: Internal server error.
- Unavailable: Server overloaded or down.
- DeadlineExceeded: Request timeout.

In all of these cases, the probability of retrying fixing something is nearly zero. For the application errors, it is intended behavior. For the server errors, the issue might not be fixed in a timely manner.

Another potential reason is running past rate limits. Retrying too soon after a ResourceExhausted error can worsen throttling.

Finally, retrying might send duplicate requests, such as making a duplicate purchase (might have got to the order succeeded stage, but something went wrong after that)

## C2
We have two main options: we can return an error, or we could return the expired data anyways.

If we return expired responses, the pros are:
- Better user experience since users get some recommendations instead of an error
- Maintains service continuity
- High resilience due to perception of reliability
Cons:
- Recommendations might be outdated.
- Content may not be as relevant.
- Users might notice and lose trust in the system.

If we return errors, the pros are:
- Ensures only up-to-date content is shown
- Clearly indicates issues to users and doesn't lie
- Simplifies logic by avoiding stale data management
Cons:
- Errors can frustrate users.
- Frequent errors can make the service seem unreliable.
- Users might disengage due to errors.

In this context, the best option, as stated in the spec, is to return the expired list of videos anyways. It is better for the user to receive something rather than nothing, even if it's the same. We can hope that the server will be back up in a short time.


## C3
One additional strategy we can implement is bringing the cache to a per-request level. We implement a local cache in VideoRecService that stores recent results from UserService and VideoService, and use this to serve requests when the services are unavailable.

Note that this only works because we are expecting users and videos to not change that much over time; people don't constantly edit their videos every few seconds. Therefore, it's particularly effective for users with relatively stable preferences and for popular videos that don't change frequently. The tradeoff of potentially outdated information is outweighed by the benefit of maintaining service availability during outages.

A rough sketch is as follows:
- Store user subscriptions, liked videos, and video metadata in memory
- Set a reasonable expiration time for cached data (eg a minute maybe)
- Update cache entries on successful service calls
- Serve from cache if primary service calls fail

This is good because it
- Reduces latency for frequent requests, as we can always fallback to cache
- Improved availability during service outages, as we still get accurate and somewhat personalized results even when the servers are down 
- Decreased load on dependent services
Cons:
- Increased memory usage in VideoRecService
- Potential for slightly outdated data
- Added complexity in managing cache consistency


## C4
I implemented the connection within the VideoRecServiceServer struct from the very start, and initialized the connection within the `MakeVideoRecServiceServer` function. Therefore, I did not create a new `Dial` or `NewClient` on every request. However, connection establishment is costly because
- each connection temporarily binds a port and ports are limited resources. Connections also use other resources such as memory, CPU, OS threads, etc.
- there is a lot of overhead in performing the SSL/TLS handshake, among other initialization steps

My implementation already avoids this by creating one client connection per service when the entire server is created, and reuses it on every connection. This leads to better resource utilization and lower latency, important for high-throughput services. In the future, we might implement things like connection pools or other mechanisms to increase the bandwidth and allow for a larger pipe.

However, there are trade-offs. For example, for load balancing, if a single connection is reused, it may not distribute the load evenly across multiple servers. We could implement connection pools or use a load balancer in front of the services can help mitigate this issue.

Some other trade-offs include:
-  Complexity for managing idle connections and their lifecycle
-  Connection pools must be sized correctly to avoid bottlenecks during traffic spikes
- a single point of failure when few connections are used, so we might need to add more robust recovery mechanisms.
- Higher latency for new connections, and managing a pool to keep connections ready is challenging.t