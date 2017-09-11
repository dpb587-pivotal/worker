# worker

Currently implemented via Concourse pipelines.

## Usage

```
$ source .envrc
$ cd src/worker
$ go run sync-pipelines.go <(cat ~/workspace/bosh-io/releases/index.yml) ../../pipelines/release-tpl.yml ../../../secrets
```

## Notes

```
 schemaname |      relname      | n_live_tup
------------+-------------------+------------
 public     | checksums         |       5083
 public     | stemcell_notes    |         50
 public     | s3_stemcells      |          1

 public     | release_notes     |       1639
 public     | jobs              |       3779
 public     | release_versions  |       3777
 public     | release_tarballs  |       3776
```

```
$ psql db-name -t -c "select convert_from(key, 'utf-8'), convert_from(value, 'utf-8') from release_versions;"| go run import-release-versions.go ~/workspace/bosh-io/releases-index/
$ psql db-name -t -c "select convert_from(key, 'utf-8'), convert_from(value, 'utf-8') from jobs;"| go run import-release-jobs.go  ~/workspace/bosh-io/releases-index/
$ psql db-name -t -c "select convert_from(key, 'utf-8'), convert_from(value, 'utf-8') from release_tarballs;"| go run import-release-tarballs.go  ~/workspace/bosh-io/releases-index/
$ psql db-name -t -c "select convert_from(key, 'utf-8'), convert_from(value, 'utf-8') from release_notes;"| go run import-release-notes.go  ~/workspace/bosh-io/releases-index/
```

## TODO

- docker image for release tpl pipeline
- use production bucket
- have dedicated ci worker
- consolidate pipelines into org pipelines
