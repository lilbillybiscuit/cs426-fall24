# Discussions for Lab 2
Name: Bill Qian

## A1

Extra Credit 1: I structured my Dockerfile as a multistage build, so the final image is only 14MB

## A3 Extra Credit
Extra credit: Properties of the UUID format depends on the UUID version.

Version 1: Based on timestamp and node ID (usually the MAC address), and provides uniqueness across time and space. This means that it is virtually impossible for two UUIDs to be the same unless it was same time, same machine

Version 4: Randomly generated using random numbers. The chance of collission is really low (about 1/2^122)

Version 5: Generated using namespace and name, using SHA-1 hashing. Same inputs will provide same UUID, but different inputs will not collide, almost like a standard hash function.

## B1

After deleting the pod and waiting a few seconds, a new pod has been created in place of the original one, although under a different name.

When a pod is deleted a Kubernetes deployment, the system automatically creates a new pod to maintain the desired state defined in the deployment configuration. This is because Kubernetes uses the Deployment controller to ensure that the specified number of replicas is always running. Each new pod is assigned a unique name to distinguish it from previous instances. (From a previous error) Furthermore, since the deployment is configured with imagePullSecrets, the new pod can access the necessary credentials to pull images from a private Docker registry.

## B4
The results are as follows:
```txt
Welcome! You have chosen user ID 202251 (Considine8488/mattbashirian@kuvalis.biz)

Their recommended videos are:
 1. The sparkly beaver's cash by Vella Kuvalis
 2. The attractive goldfish's warmth by Janae Jewess
 3. The amused mule's equipment by Ashlee Osinski
 4. Snailmight: parse by Emiliano Farrell
 5. The cruel monkey's electricity by Ali Greenholt```
```

## C3

The Grafana dashboard shows only one server (one spike) for both Video and UserService for the 1 connection-connection pool. For 4 connections, when run multiple times, sometimes we got two servers being used in equal proportion (similar graphs for both replicas), and sometimes we got them in a 1:3 ratio. For 8, the probabilty of getting 4:4 or 3:5/5:3 was far more likely.

The reason for this is the Law of Large Numbers. The more connections we make, the more likely we'll have connections that are evenly split between servers. For 4 connections, there is a 6/16 = 3/8 chance of getting an even split between both replicas of servers, while for 8 connections, there is a 70/256 ~ 27% chance of getting an even split, while getting a 3:5 or 5:3 ratio is a lot more balanced than getting 1:3 or 3:1. Therefore, by increasing the connections, we have better load balancing and distribution.