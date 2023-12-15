# Kaytu Utils

## Introduction

Share utilities between Kaytu go projects and microservices. These modules are provided
under the `/pkg` and as follows:

### `/pkg/koanf`

Load configuration from environment variables, file and default based on [koanf](https://github.com/knadh/koanf).
Following code loads configuration for a service named `testing` based on its `Config` type.

```go
cfg := koanf.Provide("testing", Config{
    RabbitMQ: koanf.RabbitMQ{
        Service:  "rabbitmq.io",
        Username: "admin",
        Password: "admin",
    },
})
```

You can pass the default values by passing an instance of the `Config` and fill it with default values.
Use `__` in the environment variables to replace `.`, for example use `RABBITMQ__SERVICE` to reference `Config.RabbitMQ.Service`.

Please note that to use `koanf` you need to tag your structure by `koanf` and set the name for configuration
as follows:

```go
type Config struct {
 RabbitMQ koanf.RabbitMQ `koanf:"rabbitmq"`
}
```
