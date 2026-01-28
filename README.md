# Propel

Propel is FeatherPanel's next-generation server control plane, built for the rapidly changing gaming industry and designed to be
highly performant and secure. Propel provides an HTTP API allowing you to interface directly with running server
instances, fetch server logs, generate backups, and control all aspects of the server lifecycle.

In addition, Propel ships with a built-in SFTP server allowing your system to remain free of Propel specific
dependencies, and allowing users to authenticate with the same credentials they would normally use to access the Panel.


## API Documentation

Swagger/OpenAPI documentation is generated from inline annotations under `router/`.

```bash
export PATH="$(go env GOPATH)/bin:$PATH" # In some distors you might need this
go install github.com/swaggo/swag/cmd/swag@latest
go generate ./router
```

The daemon serves the generated spec at `/api/docs/openapi.json`, and provides an interactive Swagger UI at `/api/docs/ui` whenever `api.docs.enabled` is `true` in `config.yml`.

## Reporting Issues

Feel free to report any propel specific issues or feature requests in [GitHub Issues](https://github.com/priyxstudio/propel/issues/new).
