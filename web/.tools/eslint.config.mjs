import js from "@eslint/js";
import globals from "globals";

export default [
  { ignores: ["web/dist/**", "**/node_modules/**"] },
  js.configs.recommended,
  {
    files: ["web/*.js"],
    ignores: ["web/*.test.js", "web/worker.js"],
    languageOptions: { sourceType: "script", globals: globals.browser },
  },
  {
    files: ["web/app.js"],
    languageOptions: { globals: { CodeMirror: "readonly" } },
  },
  {
    files: ["web/presentation.js", "web/draft-client.js", "web/release-view.js", "web/question-flow.js"],
    languageOptions: { globals: { module: "readonly" } },
  },
  {
    files: ["web/worker.js"],
    languageOptions: { sourceType: "script", globals: { ...globals.worker, Go: "readonly" } },
  },
  {
    files: ["web/*.test.js", "web/*.cjs", "web/.tools/**/*.cjs"],
    languageOptions: { sourceType: "commonjs", globals: globals.node },
  },
  {
    files: ["web/.tools/*.mjs"],
    languageOptions: { globals: globals.node },
  },
];
