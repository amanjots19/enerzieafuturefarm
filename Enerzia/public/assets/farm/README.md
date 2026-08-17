# Farm photographs

Nine photographs, nine slots on `/farm` (`app/farm/page.tsx`, copy in
`lib/content/farm.ts`). **All filled — the page has no empty slots**, and it was
restructured to be that way rather than padded out with gradients.

Two shots the page once expected were never supplied: a wide interior of the
covered hall, and the group of visitors at the pond edge. Rather than leave two
gradients sitting between real photographs, the sections that needed them were
reworked — the visit invitation now rides in the closing block, and the "why the
grade holds" section uses the biomass bowl. **If you add either photograph
later, the layout has to change back to receive it; it will not slot in on its
own.**

A slot whose file goes missing still falls back to its gradient rather than a
broken image, so a bad filename degrades quietly instead of breaking the page.

**Drop new files in using exactly the names below** — the page references them
by path, and `.jpg` is what `farm.ts` expects.

## How the current files were made

The owner supplied PNGs straight off a phone (`IMG_0541`–`IMG_0547`, ~20 MB
total, which is far too heavy for one page). They were converted with macOS
`sips` to JPEG at quality 68, longest edge 1400px, which brought the set to
2.7 MB with no visible loss:

```bash
sips -s format jpeg -s formatOptions 68 -Z 1400 IMG_0541.PNG --out harvest-filtering.jpg
```

The untouched originals are kept **outside** `public/` at
`Enerzia/assets/farm-originals/` so they are not served to browsers. Re-crop
from those rather than from the JPEGs here, and delete them if you have the
originals elsewhere.

| filename | which photograph | used by |
|---|---|---|
| `pond-raceway-wide.jpg` | The long symmetrical view down a raceway pond: blue shade-net roof, paved central walkway, four yellow paddle wheels on each side | hero |
| `pond-shadenet.jpg` | The angled view of a pond under **green** shade net, black liner wall running away from the camera, fields beyond | step 01 |
| `pond-paddlewheel.jpg` | Close on the yellow paddle wheels churning the green water | step 02 |
| `harvest-filtering.jpg` | The worker at the long white filter-cloth trough rigged across a pond, culture pouring in from the hose and foaming as it drains | step 04 |
| `harvest-biomass.jpg` | The worker in white coverall, hairnet, mask and gloves scooping thick dark-green biomass off the white filter cloth | step 04 |
| `drying-strands.jpg` | The heap of dried blue-green spirulina strands spread on white cloth. The 16:11 crop drops the shed door and background clutter, which improves it | step 05 |
| `product-jars.jpg` | Studio shot of the two tablet jars, 120 and 60, on cream with a leaf shadow. The only non-farm photograph on the page, and deliberately last — it is where the farm story hands over to the shop | closing |
| `founder.jpg` | The man in the white shirt on the walkway between the ponds tagged PP-1 and PP-2 | the byte |
| `biomass-bowl.jpg` | The steel bowl heaped with fresh deep-green biomass, held up at the pond edge with the raceway behind it | closing |

**Do not fill a missing slot with a different photograph.** Each `alt` in
`farm.ts` describes the specific shot it was written for, and the visits copy
talks about people standing at the pond edge — dropping a pond photo in there
would make both the caption and the page's own claim untrue for anyone using a
screen reader. Send the right photograph, or leave the gradient.

`.jpg` is what the table assumes. If yours are `.png` or `.webp`, either
convert them or change the `src` values in `lib/content/farm.ts` to match —
do not rename a `.png` to `.jpg`, which fixes nothing.

## Two things that will cost you an hour if ignored

**Changing a photograph? Change its filename too.** The browser caches by URL,
so replacing a file in place keeps serving the old picture — and it looks
exactly like a bad crop rather than a cache. This is recorded in
`enerzia-be/handoff.md` because it already cost a full debugging round once.

**Check the alt text still describes the picture.** Each `alt` in
`lib/content/farm.ts` describes the specific photograph it was written for. Swap
the image without swapping the words and the page starts lying to anyone using a
screen reader.

## Sizing

These are displayed at up to ~700px wide in frames of aspect ratio 4:5 (hero,
byte, closing), 16:11 (steps, visits) and 21:9 (the wide hall shot), cropped
with `object-fit: cover` — so keep the subject away from the edges. Export at
roughly 1400px on the long edge and compress — the originals off a phone are
several megabytes each and every one of them is on the critical path for this
page.
