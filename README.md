# alps

[![GoDoc](https://godoc.org/git.sr.ht/~migadu/alps?status.svg)](https://godoc.org/git.sr.ht/~migadu/alps)
[![builds.sr.ht status](https://builds.sr.ht/~migadu/alps/commits.svg)](https://builds.sr.ht/~migadu/alps/commits?)

A simple and extensible webmail.

# Usage with Mailu
This guide assumes that you are running docker compose, your mailu folder is "/mailu/" and your mailu mailu containers are using default names you also need to run some of the commands as root  
## Build the Dockerfile
Clone this repository with the docker file:

	git clone https://github.com/Willow0349/alps.git

Enter the alps folder:

    cd alps

Build the docker image:

    docker build --no-cache --pull -t alps-webmail:1.0 .

## Copy current nginx config
To export your nginx config run:

	docker cp mailu-front-1:/etc/nginx/nginx.conf /mailu/overrides/nginx/nginx.conf

## Edit exported nginx config
Now edit the nginx config `/mailu/overrides/nginx/nginx.conf` you need to change:  
`location /webmail/sso.php {` To: `location /webmail/login {`  
And  
`try_files $uri /webmail?homepage;` To: `try_files $uri /webmail;`  
Make sure to keep the indentation the same.

## Edit docker-compose.yml
Change `- "/mailu/overrides/nginx:/overrides:ro"` To: `- "/mailu/overrides/nginx/nginx.conf:/etc/nginx/nginx.conf:ro`  
And Change `image: ${DOCKER_ORG:-ghcr.io/mailu}/${DOCKER_PREFIX:-}webmail:${MAILU_VERSION:-2.0}` To: `image: alps-webmail:1.0`
Again make sure to keep the indentation the same.

## Reload Mailu

    docker compose down && docker compose up -d
