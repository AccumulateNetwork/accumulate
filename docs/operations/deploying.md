# Deploying a new version of the protocol

## 1. Build an image

### Option 1

Create and push a tag and wait for CI to build the image, for example:

```shell
git tag -a v1.3.0-rc.3.5 -m 'Release 1.3.0 release candidate 3.5'
git push --tags
```

### Option 2

Manually build and push an image, for example:

```shell
make docker-push IMAGE=registry.gitlab.com/accumulatenetwork/accumulate:v1-3-0-rc-3-5
```

**Note:** For consistency with CI builds, replace `.` (and any other
non-alphanumerics) with `-`.

**Note:** Always use `make docker-push` for this to ensure the version fields
are set correctly.

## 2. Update network map (for AccMan)

Open [networkmap.yaml][nmap] (in the network-map project), and under the target
network update `Tags` or `DevTags` to include the new tag.

**Note**: `Tags` and `DevTags` support commas and wildcards (`*`) but the
easiest way is to only have one tag so there's zero ambiguity when restarting
AccMan.

[nmap]: https://gitlab.com/accumulatenetwork/network-map/-/blob/main/networkmap.yaml

## 3. Deploy the new version

For AccMan, stop and start with the new version. Wait for all the nodes to boot
up.

An easy way to launch AccMan: `ssh <host> -t '~/accman/accman'`

**Note:** Especially on Kermit, if a server has been down for a while, the API
gateway may need to be restarted.

## 4. Activate

If the update includes protocol changes, send the activation transaction, for
example:

```shell
accumulate tx execute dn.acme '{ type: activateProtocolVersion, version: v2Baikonur }'
```