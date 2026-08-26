/**
 * Renders one schema.org JSON-LD block.
 *
 * A server component with no `'use client'`, so the markup is in the HTML a
 * crawler receives on the first request rather than after hydration — which is
 * the entire point of emitting structured data.
 *
 * ---------------------------------------------------------------------------
 * ABOUT THE `dangerouslySetInnerHTML`
 *
 * There is no safe alternative. React escapes text children as HTML entities,
 * and `{"` inside a `<script>` becomes `{&quot;`, which is not JSON and which
 * every validator rejects. Every JSON-LD implementation writes it this way.
 *
 * What makes it safe here is the escaping below, not the absence of the API:
 * `JSON.stringify` cannot emit a raw `<`, but it CAN emit the three-character
 * sequence `</s` inside a string value — and a product name containing
 * `</script>` would otherwise close this tag early and turn the rest of the
 * payload into live markup. Product names come from the manager's catalogue,
 * so they are not attacker-controlled, but they are also not reviewed by
 * anyone here. Escaping `<` to its `<` JSON escape closes that off
 * permanently: it is still the same string once parsed, and it can no longer
 * terminate the element.
 * ---------------------------------------------------------------------------
 */
export function JsonLd({ data }: { data: Record<string, unknown> }) {
  const json = JSON.stringify(data).replace(/</g, '\\u003c');

  return (
    <script
      type="application/ld+json"
      // eslint-disable-next-line react/no-danger
      dangerouslySetInnerHTML={{ __html: json }}
    />
  );
}
