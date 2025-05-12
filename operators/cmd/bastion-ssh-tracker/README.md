# Bastion SSH Tracker

To be added to ssh-bastion deployment as sidecar container together with the bastion-operator.

TODO: add it to deployment manifest.

Golang app based on Google `gopacket` to track SSH connections going from the bastion host to the target host.
It will track the SSH connections and expose metrics to Prometheus.

It needs to be build with a custom container image that has `libpcap-dev` installed and statically build with an alpine image. Moreover, since `gopacket` relies on C libraries (`libpcap`), we need to build the binary with `CGO_ENABLED=1`.

```bash
cd operators
docker build -t ssh-tracker -f build/ssh-tracker/Dockerfile --build-arg COMPONENT=bastion-ssh-tracker .
docker run --rm -it --cap-add=NET_RAW --cap-add=NET_ADMIN ssh-tracker -i any -port 22
```

Note: In order to set `NET_RAW NET_ADMIN` in K8S, you need to set the `securityContext` in the deployment manifest.
```yaml
securityContext:
  capabilities:
    add:
      - NET_ADMIN
      - NET_RAW
```

## TODO
- Expose metrics to Prometheus
- Reverse lookup to get the hostname of the target host
