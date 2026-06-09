import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import jsxA11y from "eslint-plugin-jsx-a11y";
import tseslint from "typescript-eslint";
import { defineConfig, globalIgnores } from "eslint/config";
import stylistic from "@stylistic/eslint-plugin";

export default defineConfig([
  globalIgnores(["dist"]),
  {
    files: ["**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
      stylistic.configs.recommended,
      jsxA11y.flatConfigs.recommended,
    ],
    languageOptions: {
      globals: globals.browser,
    },
    rules: {
      // Add/override style rules here
      "@stylistic/comma-dangle": ["error", "always-multiline"],
      "@stylistic/object-curly-spacing": ["error", "always"],
      "@stylistic/array-bracket-spacing": ["error", "never"],
      "@stylistic/arrow-parens": ["error", "always"],
      "@stylistic/max-len": [
        "warn",
        {
          code: 120,
          ignoreUrls: true,
          ignoreStrings: true,
          ignoreTemplateLiterals: true,
        },
      ],
    },
  },
  {
    // a11y backlog: these legacy raw-HTML surfaces predate jsx-a11y enforcement
    // (div-with-onClick that needs keyboard support). The two interaction rules
    // are relaxed here ONLY — everywhere else (incl. all new primitives) stays
    // enforced. Remove each entry as its tab is migrated to the new, a11y-clean
    // primitives + Shell, so this list ratchets down to empty.
    files: [
      "src/tabs/PlayersTab/modals/GiveItemsModal.tsx",
      "src/tabs/PlayersTab/modals/ManagePacksModal.tsx",
      "src/tabs/PlayersTab/views/ActionsView/components/AddTagsPanel.tsx",
      "src/tabs/PlayersTab/views/GiveItemsView.tsx",
      "src/tabs/ServerSettingsTab/components/RawSectionPanel.tsx",
      "src/tabs/StorageTab/components/AddItemsModal.tsx",
      "src/tabs/WelcomePackageTab/views/PackagesView.tsx",
    ],
    rules: {
      "jsx-a11y/click-events-have-key-events": "off",
      "jsx-a11y/no-static-element-interactions": "off",
    },
  },
]);
