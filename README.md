# Concourse Spinnaker Resource

## Features :
   - Trigger a Spinnaker pipeline with optional artifacts from Concourse.
   - Trigger a Concourse job based on the status of Concourse type stage of a Spinnaker pipeline.

## Source Configuration

- `spinnaker_api`: *Required* the url of the Spinnaker api microservice.
- `spinnaker_application`: *Required* The Spinnaker application you would like to trigger.
- `spinnaker_pipeline`: *Required* The Spinnaker pipeline you would like to trigger.
- `spinnaker_x509_cert`: *Required* Client [certificate](https://www.spinnaker.io/setup/security/authentication/x509/) to authenticate with Spinnaker.
- `spinnaker_x509_key`: *Required* Client [key](https://www.spinnaker.io/setup/security/authentication/x509/) to authenticate with Spinnaker.
- `statuses`: *Optional* Array of Spinnaker pipeline concourse stage statuses. Currently supported statuses by Spinnaker: [NOT_STARTED, RUNNING, PAUSED, SUSPENDED, SUCCEEDED, FAILED_CONTINUE, TERMINAL, CANCELED, REDIRECT, STOPPED, SKIPPED, BUFFERED] - [Reference](https://github.com/spinnaker/gate/blob/1cb00104f925e484d7a7a333bf07bd149adb0464/gate-web/src/main/groovy/com/netflix/spinnaker/gate/controllers/ExecutionsController.java#L82).
   - if specified, the status will be used to filter the pipeline concourse stage execution statuses when detecting new versions during the `check` step.
   - if specified ,the `put` step will block until the specified status(es) is reached.
- `statuses_check_timeout`: *Optional* The amount of time after which the `put` step will timeout waiting for the `statuses`. Default value will be `30m`.

## Behaviour

### `check`

Pipeline executions will be found by fetching pipeline executions for the configured application, filtered by the pipeline name. If `statuses` is configured, the list will be filtered by statuses.

The pipeline execution `id` will be used as the version of the resource.

API : `GET /applications/{application}/pipelines`

### `in`

Places the following files in the destination:

 - `metadata.json`: Contains the pipeline execution metadata returned from the Spinnaker [API](https://www.spinnaker.io/reference/api/docs.html#api-Pipelinecontroller-getPipelineUsingGET).

 - `version`: A file containing the pipeline execution id.

 API : `GET /pipelines/{id}`

### `out`: Triggers a pipeline

Triggers a Spinnaker pipeline.

#### Parameters

- `artifacts_json_file`: *Optional* path to a file containing the artifacts to trigger the spinnaker pipeline with. File should contain an array of artifacts in JSON format to trigger along with the pipeline in the [spinnaker artifact format](https://www.spinnaker.io/reference/artifacts/#format). 

- `trigger_params`: *Optional* build information to send to Spinnaker pipeline execution which can be consumed by the [pipeline expressions](https://www.spinnaker.io/guides/user/pipeline-expressions/). Can be any key/value pair. Any [metadata](http://concourse.ci/implementing-resources.html#resource-metadata) will be evaluated prior to triggering the pipeline.

- `trigger_params_json_file`: *Optional* Path to a file that contains parameters to push to the Spinnaker pipeline. This allows the file to be generated by a previous task step. Contents of this file will be merged with `trigger_params` with the file getting precedence.

## Example Pipelines

### Put
```yml
---
resource_types:
- name: spinnaker
  type: docker-image
  source:
    repository: concourse/spinnaker-resource

- name: trigger-spinnaker-pipeline
  type: spinnaker
  source:
    spinnaker_api: https://api.spincon.ci.cf-app.com:8085
    spinnaker_application: nvidia
    spinnaker_pipeline: deploy
    spinnaker_x509_cert: ((spinnaker_x509_cert))
    spinnaker_x509_key: ((spinnaker_x509_key))
    status_check_timeout: 2m
    statuses:
    - succeeded

jobs:
- name: trigger-pipeline
  plan:
  - put: trigger-spinnaker-pipeline
    params:
      trigger_params:
        build_id: (build ${BUILD_ID})
      artifacts_json_file: some-other-resource/artifact.json
      trigger_params_json_file: some-task-output/params.json
```


### Get
```yml
---
resource_types:
- name: spinnaker
  type: docker-image
  source:
    repository: concourse/spinnaker-resource

resources:
  - name: listen-on-spinnaker-executions
    type: spinnaker
    source:
      spinnaker_api: ((spinnaker-api))
      spinnaker_x509_cert: ((client-x509-cert))
      spinnaker_x509_key: ((client-x509-key))
      spinnaker_application: samplespinnakerapp
      spinnaker_pipeline: samplespinnakerpipeline
      statuses:
        - SUCCEEDED
        - TERMINAL

jobs:
- name: trigger-pipeline
  plan:
  - get: listen-on-spinnaker-executions
    trigger: true
```
