# Archived image publishing workflows

The alpha and nightly workflows are retained here for project history but are
inactive because GitHub Actions only loads workflow files from
`.github/workflows/`.

The active ECR publishing model has two environments:

- `Deploy Test` publishes the `test` tag.
- `Publish Prod` publishes the `prod` tag.

Both active workflows accept `us`, `cn`, or `both` as the target region.
