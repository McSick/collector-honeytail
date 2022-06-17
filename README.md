## Quickstart

```
export HONEYCOMB_API_KEY=<your-honeycomb-key>
export HONEYCOMB_LOGS_DATASET=<your-logs-dataset-name>
export HONEYCOMB_METRICS_DATASET=<your-metrics-dataset-name>
export HONEYCOMB_TRACING_DATASET=<your-tracing-dataset-name>
docker-compose up -d --build
```

Notes: 

- HONEYCOMB_TRACING_DATASET is only required if you are on Honeycomb Classic. If you are not, you can remove this and also adjust the otel-local-config file to not use dataset for tracing exporter as this will be inferred from the service.name field.  
- The Dockerfile downloads the AMD version of Honeytail.  See [docs](https://docs.honeycomb.io/getting-data-in/logs/honeytail/#installation) for other versions and SHA's.

## Thanks  
The majority of the work was done by Sasha taken from [here](https://github.com/sgsharma/collector-honeytail-logs)
