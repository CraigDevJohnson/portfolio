/** @type {import('stylelint').Config} */
export default {
  extends: ["stylelint-config-standard"],
  rules: {
    "selector-class-pattern": "^[a-z]([a-z0-9-]*)(?:--[a-z0-9-]+)?$"
  },
  reportInvalidScopeDisables: true,
  reportNeedlessDisables: true
};
