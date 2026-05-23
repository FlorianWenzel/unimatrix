// Shared support file loaded before every spec.
// Keep this minimal — feature-specific helpers belong in the spec or
// in a focused command in cypress/support/commands.js (added when needed).

Cypress.on('uncaught:exception', (err) => {
  // The Borg interface intentionally throws no client-side errors;
  // if it does, fail the test loudly.
  // eslint-disable-next-line no-console
  console.error('uncaught exception in app', err);
  return true;
});
