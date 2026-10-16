# One service's database migrations: the migrate CLI plus that service's SQL files. CI builds it
# next to the service image, with the same tag, using the service's migrations directory as the
# build context. Nothing runs it automatically - rekreativko-infra's `task migrate` runs it by hand.
ARG MIGRATE_VERSION=v4.19.1
FROM migrate/migrate:${MIGRATE_VERSION}

COPY *.sql /migrations/

USER 65532:65532

# ENTRYPOINT is the base image's migrate CLI; the caller passes -path=/migrations -database=... up
