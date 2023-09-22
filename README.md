**btidor/syntax** is a syntax extension that makes it easier and faster to run
`apt-get install` in Dockerfiles. By properly caching downloaded files and
surfacing them in the Docker build DAG, it can reduce build times by 50%.

To use, add this magic comment to the top of your Dockerfile...

```docker
# syntax = btidor/syntax
```

...and install packages like so:

```docker
ADD --apt clang nginx sl
```

## Background

The [Docker-recommended][1] way to install packages in an image is:

```docker
RUN apt-get update && \
    apt-get install -y clang nginx sl && \
    rm -rf /var/lib/apt/lists/*
```

This approach has a few drawbacks:

1. There's no caching or sharing across builds. Base images are stripped to save
   space, so `apt-get update` must download the entire package index every time
   the layer is rebuilt (Ubuntu's index is 27 MB on the wire). Then, `apt-get
   install` redownloads the packages themselves &mdash; 176 MB in this example.

2. By default, apt saves both the package files and the index files on disk. The
   official Debian and Ubuntu images include a configuration file,
   `/etc/apt/apt.conf.d/docker-clean`, which deletes the package files after
   installation completes (saving 249 MB in this example). Unfortunately, we
   have to manually clean up the index files with `rm -rf /var/lib/apt/lists/*`,
   otherwise they bloat the final layer by 44 MB.

The [more][2] [advanced][3] [method][4] of installing packages in a Docker image
is to store the package files in a cache mount, which is shared between all
builds on the host:

```docker
RUN --mount=type=cache,target=/var/cache/apt \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    apt-get update && \
    apt-get install -y clang nginx sl && \
    rm -rf /var/lib/apt/lists*
```

By caching the large package downloads, this approach provides much better
performance. However, there are still a few issues:

1. We _definitely_ have to remember to remove the `docker-clean` script. Because
   the cache mount is shared globally, any Dockerfile that forgets this will
   trash the cache for everyone.

2. The cache mount can be unreliable. Docker isn't able to evict individual
   files from the mount, so it grows steadily in size until the entire mount is
   evicted, at which point packages are redownloaded and the cycle repeats.

   What we'd really like to do is route the downloads through Docker's HTTP
   cache, which provides file-level granularity and excellent parallelism. As a
   bonus, creating a clean build with the `--no-cache` flag bypasses cache
   mounts but can still hit the HTTP cache, which uses ETags and checksums to
   stay fresh.

3. This method doesn't cache index files, which would save another few seconds
   in `apt-get update`.

4. It's pretty verbose. A simpler syntax would be nice.

This syntax extension uses the following strategy:

1. Run `apt-get update` with a shared cache for the index files. If the cache
   is fresh, this step can take under a second.

2. Run `apt-get install --print-uris`, which produces the list of packages apt
   would have downloaded during the install step.

3. Convert those URIs to `ADD` instructions. If apt provides a SHA-256 hash, the
   package can be served from the cache without ever hitting the server.
   Otherwise, Docker uses ETags to avoid unnecessary redownloads.

4. The apt state is assembled in a temporary layer that's mounted into the
   container. We point apt at the mount point and run `apt-get install` to
   perform the final installation. The state layer is then unmounted and
   discarded, leaving only a pair of zero-byte placeholder entries in the image
   history.

```docker
$ docker history 25da9a5d2bbc
IMAGE          CREATED         CREATED BY                                      SIZE      COMMENT
25da9a5d2bbc   8 seconds ago   [4/4] ADD (apt install) clang nginx sl # bui…   893MB     buildkit.dockerfile.v0
<missing>      8 seconds ago   [3/4] ADD (apt download) clang nginx sl # bu…   0B        buildkit.dockerfile.v0
<missing>      8 seconds ago   [2/4] ADD (apt update) clang nginx sl # buil…   0B        buildkit.dockerfile.v0
<missing>      4 weeks ago     /bin/sh -c #(nop)  CMD ["/bin/bash"]            0B
<missing>      4 weeks ago     /bin/sh -c #(nop) ADD file:aa9b51e9f0067860c…   77.8MB
<missing>      4 weeks ago     /bin/sh -c #(nop)  LABEL org.opencontainers.…   0B
<missing>      4 weeks ago     /bin/sh -c #(nop)  LABEL org.opencontainers.…   0B
<missing>      4 weeks ago     /bin/sh -c #(nop)  ARG LAUNCHPAD_BUILD_ARCH     0B
<missing>      4 weeks ago     /bin/sh -c #(nop)  ARG RELEASE                  0B
```

## Details

This syntax extension supports any base image that includes apt in `$PATH`. The
system's configured sources are used, which can include third-party
repositories:

```docker
# syntax = btidor/syntax

FROM ubuntu
COPY nodesource.sources /etc/apt/sources.list.d/
ADD --apt nodejs
```

Note that when the `--apt` flag is passed, any other flags to the `ADD`
instruction are ignored.

This extension calls `apt-get` instead of `apt`, since `apt` [is not meant to be
used in scripts][5].

Some documentation recommends pinning specific package versions to improve
reproducibility. That's probably not a great idea, since mirrors often remove
outdated versions to save space.

Fun fact: Docker syntax extensions are shipped as Docker containers! Adding the
magic comment at the top of a Dockerfile causes Docker to download and run the
[btidor/syntax][6] image. The image contains a single, statically-linked Go
binary that runs a gRPC server that takes the Dockerfile as input and turns it
into a dependency graph in [LLB][7], BuildKit's intermediate representation.

[1]: https://docs.docker.com/develop/develop-images/dockerfile_best-practices/#apt-get
[2]: https://docs.docker.com/build/cache/#use-the-dedicated-run-cache
[3]: https://vsupalov.com/buildkit-cache-mount-dockerfile/
[4]: https://depot.dev/blog/how-to-use-buildkit-cache-mounts-in-ci
[5]: https://manpages.ubuntu.com/manpages/xenial/man8/apt.8.html#script%20usage%20and%20differences%20from%20other%20apt%20tools
[6]: https://hub.docker.com/r/btidor/syntax
[7]: https://github.com/moby/buildkit/blob/master/docs/dev/dockerfile-llb.md
