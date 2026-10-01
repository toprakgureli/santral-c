import js from "@eslint/js";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

// The panel's lint rules: the recommended JavaScript and TypeScript checks
// plus the two hook rules, kept strict because a broken hook order or a stale
// value inside an effect is a real bug in a page agents keep open all day.
export default tseslint.config(
  { ignores: ["dist", "node_modules"] },
  {
    files: ["**/*.{ts,tsx,js}"],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    languageOptions: {
      ecmaVersion: 2020,
      globals: { ...globals.browser, ...globals.node },
    },
    plugins: { "react-hooks": reactHooks },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
      // A callback may read a variable before the line that assigns it, and
      // const would throw there where let reads undefined.
      "prefer-const": ["error", { ignoreReadBeforeAssign: true }],
    },
  },
);
