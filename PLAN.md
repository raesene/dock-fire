# Docker firecracker integration

The plan for this project is to allow Docker to use firecracker when creating new containers, providing improved isolation.

This follows a similar pattern to gvisor which can work with Docker (documentation here https://gvisor.dev/docs/user_guide/quick_start/docker/)

When complete, a user should be able to pass a `--runtime` flag to invoke a Firecracker VM as the runtime for a Docker container.

when testing the code you can SSH to 192.168.41.108 as the user rorym and run any required build and test commands on that host.
