## proof-of-concept

restricted APT semi-proxy repository via Sonatype Nexus

- store (upstream) package lists in separate repository (with no modification)
- store allowed/approved packages in separate repository
- single HTTP(s) redirector service

pros:

- APT package lists are pristine and sane
- packages are strictly limited
- reuse upstream/distro GPG keyrings - no need to redistribute own GPG keyring(s) which are used to sign (own) hosted APT repository

cons:

- requires periodic package list sync
- requires manual package upload into hosted repositories

## considerations

- existing Sonatype Nexus instance/cluster is available at `https://nexus.svc.domain.com`
- "apt-nexus-redirector" will be available at `https://apt.svc.domain.com`

## how-to

- Sonatype Nexus:

  - setup repositories for each distro/suite you willing to host/proxy:

    - hosted raw: `raw-lists-${distro}-${suite}`
    - hosted apt: `apt-${distro}-${suite}`

    e.g. for Debian 12 "Bookworm":

    - hosted raw: `raw-lists-debian-bookworm`
    - hosted apt: `apt-debian-bookworm`

    **NB**: suites like `bookworm-updates` (and so on) must be set separately

  - sync APT repo lists from upstream archive/mirror via [`sync-lists.sh`](sync-lists.sh):

    `sync-lists.sh ${distro} ${suite}`

    e.g. for Debian 12 "Bookworm":

    `sync-lists.sh debian bookworm`

    **NB**: suites like `bookworm-updates` (and so on) must be synced separately

    `sync-lists.sh debian bookworm-updates`

  - upload selected APT packages to `apt-${distro}-${suite}` repositories

    there's no script (yet). contributions are welcome.

- `apt-nexus-redirector`:

  - build container image with [Dockerfile](apt-nexus-redirector/Dockerfile)
  - deploy container image with "apt-nexus-redirector"

- target setup:

  - setup `/etc/apt/sources.list` accordingly

    e.g. for Debian 12 "Bookworm":

    `deb https://apt.svc.domain.com/debian/bookworm bookworm main`

    **NB**: suites like `bookworm-updates` (and so on) must be specified in that way:

    `deb https://apt.svc.domain.com/debian/bookworm-updates bookworm-updates main`

    In case of configuration error like this:

    `deb https://apt.svc.domain.com/debian/bookworm bookworm-updates main`

    `apt-nexus-redirector` will respond with HTTP 400 and set header `X-Location-Proposal` with proposed location.
