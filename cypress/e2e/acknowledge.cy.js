// End-to-end test for the "acknowledge" (like) action on a transmission.
//
// As of the time this spec was written, the backend route POST /like
// exists and works, but no template renders a button that posts to it —
// see issue #91. This test is expected to FAIL until the UI is shipped.
//
// The selectors are intentionally permissive ("acknowledge" anywhere in
// a button's text) so the dev team has flexibility in how to label the
// control, while still catching the case where no control exists at all.

describe('acknowledge a transmission', () => {
  it('lets a logged-in drone acknowledge a transmission and reflects it in the count', () => {
    // Generate a unique designation INSIDE the test body so Cypress
    // retries get a fresh designation each attempt. Otherwise a retry
    // hits an "already exists" error on /register and masks the real
    // failure further down the spec.
    const designation = `tester${Date.now()}${Math.floor(Math.random() * 1e6)}`;
    const accessCode = 'resistance-is-futile';
    const body = 'first contact transmission';
    // Register a drone.
    cy.visit('/register');
    cy.get('input[name="designation"]').type(designation);
    cy.get('input[name="access_code"]').type(accessCode);
    cy.contains('button', /assimilate/i).click();

    // Land on home, post a transmission.
    cy.url().should('match', /\/$|\/?$/);
    cy.get('textarea[name="body"]').type(body);
    cy.contains('button', /transmit/i).click();

    // Locate the transmission card just posted. Anchor to the body text
    // since IDs are not exposed in a stable, queryable form to the user.
    cy.contains('.transmission, [class*="transmission"]', body)
      .as('post')
      .should('be.visible');

    // Before acknowledging, the count line should not say "1
    // acknowledgments" (template suppresses the line when Likes==0).
    cy.get('@post').should('not.contain', '1 acknowledgments');

    // Verify the button shows unacknowledged state with "♡ acknowledge"
    cy.get('@post')
      .contains('button', /acknowledge|^ack$|\back\b/i)
      .as('ackButton')
      .should('contain', '♡ acknowledge');

    // Click the acknowledge control.
    cy.get('@ackButton').click();

    // After acknowledging, reload the page and assert the count reflects
    // the acknowledgment and the button now shows acknowledged state.
    cy.reload();
    cy.contains('.transmission, [class*="transmission"]', body)
      .as('postAfterAck')
      .should('contain', '1 acknowledgments');

    // Verify the button now shows acknowledged state with "♥ acknowledged"
    cy.get('@postAfterAck')
      .contains('button', /acknowledge|^ack$|\back\b/i)
      .should('contain', '♥ acknowledged');
  });
});
