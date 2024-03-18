# Usage with Mailu
## Copy current nginx config
Assuming you are running docker compose, your mailu folder is "/mailu/" and your mailu nginx is named "mailu-front-1"  
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
Again make sure to keep the indentation the same.


# [alps]

[![GoDoc](https://godoc.org/git.sr.ht/~migadu/alps?status.svg)](https://godoc.org/git.sr.ht/~migadu/alps)
[![builds.sr.ht status](https://builds.sr.ht/~migadu/alps/commits.svg)](https://builds.sr.ht/~migadu/alps/commits?)

A simple and extensible webmail.

## Usage

Assuming SRV DNS records are properly set up (see [RFC 6186]):

    go run ./cmd/alps example.org

To manually specify upstream servers:

    go run ./cmd/alps imaps://mail.example.org:993 smtps://mail.example.org:465

Add `-theme alps` to use the alps theme. See `docs/cli.md` for more
information.

When developing themes and plugins, the script `contrib/hotreload.sh` can be
used to automatically reload alps on file changes.

## Contributing

Send patches on the [mailing list], report bugs on the [issue tracker].

## License

MIT

[alps]: https://sr.ht/~migadu/alps
[RFC 6186]: https://tools.ietf.org/html/rfc6186
[Go plugin helpers]: https://godoc.org/git.sr.ht/~migadu/alps#GoPlugin
[mailing list]: https://lists.sr.ht/~migadu/alps-devel
[issue tracker]: https://todo.sr.ht/~migadu/alps
