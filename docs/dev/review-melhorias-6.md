# review-melhorias-6 (§12)

- .github/workflows/release-windows.yml:41 — `npm run dist:win` runs electron-builder w/o `--publish`; in CI it auto-publishes (release event: GITHUB_REF_TYPE=tag → "onTag"; workflow_dispatch: isCI → "onTagOrDraft"; see node_modules/app-builder-lib/out/publish/PublishManager.js:44-64), GitHub provider inferred from git remote, GH_TOKEN not set in that step → build fails "GitHub Personal Access Token is not set" (or, if token present, races linux job on same release) — fix: `run: npm run dist:win -- --publish never` (mirrors linux `--publish never`).

item 25 (card model persisted): LGTM (cmds.go:463-477, spawn.go:139-145, store.go:651, src/store.ts:256, src/App.tsx:133 round-trip ok).
