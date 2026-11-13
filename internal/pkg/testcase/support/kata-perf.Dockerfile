ARG BASE
FROM ${BASE} AS fixture
ARG FIXTURE_ID
RUN test -n "$FIXTURE_ID" && printf '%s' "$FIXTURE_ID" > /kata-fixture-id

FROM scratch
COPY --from=fixture / /
USER 1000:1000
CMD ["sh"]
