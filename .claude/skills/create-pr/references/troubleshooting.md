# Create-PR Troubleshooting

## Common Issues

### 1. "gh: command not found"
**Solution**: Install GitHub CLI
```bash
brew install gh
gh auth login
```

### 2. "no upstream branch"
**Solution**: Skill will automatically push with `-u` flag

### 3. PR description too long
**Solution**: Skill will summarize commits; user can edit before creation

### 4. Wrong base branch detected
**Solution**: User can change base branch in Step 4, or set `base_branch` in `config.project.yaml`

### 5. Missing context in description
**Solution**: Add detailed commit messages; use `smart-commit` skill

## Configuration

The `config.project.yaml` file in this skill directory controls project-specific behavior.
See `config.project.yaml` for the full schema and defaults.

Key settings:
- `base_branch`: Target branch for PRs
- `template.language`: PR description language (`"en"` or `"ja"`)
- `template.sections`: Section names and behavior
- `checklist.default`: Always-shown checklist items
- `checklist.conditional`: Items shown based on detected file changes
- `detection`: File patterns to detect schema, query, UI, test changes
- `layers`: Architecture layers for impact analysis

## Layer Detection

The skill auto-detects affected layers from `config.project.yaml` → `layers`.
If no layers are configured, use directory-level grouping:

| Pattern | Impact Level |
|---------|-------------|
| Core domain / entity files | High |
| Service / use case files | High |
| Repository / data access | Medium |
| Controllers / handlers | Medium |
| Config / DI | Low |
| Tests | Info |
| Docs | Info |
