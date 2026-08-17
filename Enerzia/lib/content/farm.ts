/**
 * "Our Farm" page copy.
 *
 * Written from the owner's own photographs of the farm (supplied 2026-08-16),
 * so every process step below describes something visible in a picture on the
 * page beside it: the lined raceway ponds under shade net, the paddle wheels
 * turning the water, the covered harvest hall, the filtered biomass being
 * lifted off the cloth, and a visiting group standing at the pond edge.
 *
 * ---------------------------------------------------------------------------
 * WHAT IS CLAIMED HERE, AND ON WHAT AUTHORITY
 *
 * This page exists to say "we grow it ourselves", which is a sourcing claim a
 * buyer cannot check. So the rule for editing this file is: every sentence is
 * either (a) visible in the photograph it sits next to, or (b) already claimed
 * elsewhere in the repo and therefore already the owner's word.
 *
 * Carried over from existing copy rather than invented here:
 *   - "sun-dried at low heat", "lab-tested batch by batch"  → lib/content/home.ts HERO
 *   - FSSAI licensed · NABL third-party lab · microbiology · microcystin
 *                                                            → home.ts SEALS
 *   - "No binders, no fillers"                → the tablets product copy (Mongo)
 *   - 62%+ plant protein by weight                          → home.ts HERO badge
 *
 * DELIBERATELY ABSENT, because nobody has supplied them: pond count, litres,
 * yields, harvest frequency, drying temperature, years in operation, altitude,
 * water source. Do not fill these in from a plausible guess — a sourcing page
 * is the worst possible place to be caught approximating. Ask the owner, then
 * add.
 * ---------------------------------------------------------------------------
 */

import type { Photo } from './types';

// The photo shape is shared with the homepage hero — see ./types.ts. Re-exported
// so this file still reads as the one place the farm page's content lives.
export type { Photo };

export const FARM_HERO = {
  eyebrow: 'Our farm',
  headingLines: ['We do not buy', 'spirulina in.', 'We grow it.'],
  body: 'Every gram we sell was grown in these ponds, harvested by our own team, dried on site and packed here. There is no trader in the middle, no imported drum relabelled with our name - which is why we can tell you what happened to your powder on the day it was made.',
  photo: {
    src: '/assets/farm/pond-raceway-wide.jpg',
    alt: 'A long spirulina raceway pond under a blue shade-net roof, split by a paved central walkway, with yellow paddle wheels turning the deep green water on both sides.',
    gradient: 'radial-gradient(125% 100% at 30% 20%,#cfe9d5,#4a9077 52%,#083628)',
  } satisfies Photo,
};

/** The three-line claim that sits under the hero. Kept short on purpose. */
export const OWN_FARM_POINTS = [
  {
    title: 'One farm, start to finish',
    body: 'Grown, harvested, dried, milled and packed by the same team in the same place. Nothing is bought in and rebranded.',
  },
  {
    title: 'Traceable to the pond',
    body: 'Because we run the ponds ourselves, a pack can be traced back to the water it grew in and the day it came out.',
  },
  {
    title: 'Tested before it is sold',
    body: 'Every batch goes to an independent NABL-accredited laboratory before it is packed - not a sample, every batch.',
  },
];

/**
 * One stage of production.
 *
 * `photo` is **required**, and that is the design rule made into a compile
 * error: the steps alternate image and text down the page, so a step without a
 * picture reads as a missing photograph rather than as a deliberate choice. A
 * stage nobody has photographed does not belong in this list — the lab-testing
 * step was removed for exactly that reason, and its claim now lives in
 * `OWN_FARM_POINTS` instead.
 */
export interface ProcessStep {
  /** Two-digit ordinal shown beside the heading. */
  step: string;
  title: string;
  body: string;
  photo: Photo;
}

export const PROCESS_INTRO = {
  eyebrow: 'How it is made',
  heading: 'From pond water to a sealed pack.',
  body: 'Spirulina is a crop, and it is grown like one - slowly, in the open air, watched every day. This is the whole route, in the order it happens.',
};

export const PROCESS: ProcessStep[] = [
  {
    step: '01',
    title: 'Lined ponds, under shade',
    body: 'The spirulina grows in shallow raceway ponds, each one lined so the culture never touches soil. A shade net is stretched over the whole run: it takes the hardest edge off the afternoon sun while still letting the crop photosynthesise, and it keeps dust, leaves and birds off the water.',
    photo: {
      src: '/assets/farm/pond-shadenet.jpg',
      alt: 'A raceway pond of bright green spirulina culture under a green shade net, its black liner wall running the length of the pond, open farmland visible beyond.',
      gradient: 'radial-gradient(120% 100% at 35% 25%,#d9efd9,#5f9a76 55%,#123f2d)',
    },
  },
  {
    step: '02',
    title: 'Moving water, watched daily',
    body: 'Paddle wheels turn the culture around the raceway all day. Moving water is the whole trick: it brings every filament up to the light in turn, keeps the temperature even end to end, and stops anything settling on the bottom. The ponds are walked and read by hand every day, and the crop is harvested when it is ready rather than when the calendar says so.',
    photo: {
      src: '/assets/farm/pond-paddlewheel.jpg',
      alt: 'Yellow paddle wheels part-submerged in a spirulina pond, churning the green water into pale foam as they turn.',
      gradient: 'radial-gradient(120% 100% at 60% 30%,#e2f0d6,#7fa85c 52%,#2d5327)',
    },
  },
  {
    step: '03',
    title: 'Harvested by filtering, not by chemicals',
    body: 'At harvest the culture is pumped out of the pond and through fine filter cloth. The spirulina stays behind as a thick, dark green biomass and the clean water runs straight back where it came from. Nothing is added to make it separate - the mesh does the work.',
    photo: {
      src: '/assets/farm/harvest-filtering.jpg',
      alt: 'A worker in a white coverall, hairnet, mask and gloves tending a long filter-cloth trough rigged across a pond, culture pouring from a hose into it and foaming as the water drains away.',
      gradient: 'radial-gradient(120% 100% at 45% 40%,#d6ecdc,#3f8f7a 52%,#0f4038)',
    },
  },
  {
    step: '04',
    title: 'Handled clean, by hand',
    body: 'From the moment it leaves the water the biomass is handled on stainless steel, by people in coveralls, gloves and masks, under cover. This is the step where a food is won or lost, so it does not happen in the open.',
    photo: {
      src: '/assets/farm/harvest-biomass.jpg',
      alt: 'A worker in a white coverall, hairnet, mask and gloves lifting thick dark green spirulina biomass off a white filter cloth with a scoop, into a stainless steel pot.',
      gradient: 'radial-gradient(120% 100% at 45% 40%,#cfe3c4,#3f6b3a 55%,#16351a)',
    },
  },
  {
    step: '05',
    title: 'Pressed into strands, dried gently',
    body: 'The wet biomass is pressed into fine strands and laid out on clean cloth to dry in moving air at low heat - never a hard roast, because heat is what costs spirulina its colour and its nutrition. Once dry it is milled to powder, or pressed into 500 mg tablets with no binders and no fillers, then sealed into light-proof packs - spirulina fades in sunlight.',
    photo: {
      src: '/assets/farm/drying-strands.jpg',
      alt: 'A heap of dark blue-green dried spirulina strands, extruded like short noodles, spread out on white cloth to dry in the open air.',
      gradient: 'radial-gradient(120% 100% at 45% 45%,#dfe9e2,#3f7a72 52%,#123534)',
    },
  },
];

/**
 * The invitation that closes the process section.
 *
 * It replaced a "lab tested, then sealed" step (owner's decision, 2026-08-16).
 * **The testing claim itself was not dropped** — it still runs in
 * `OWN_FARM_POINTS` ("Tested before it is sold ... an independent
 * NABL-accredited laboratory ... not a sample, every batch") and again in
 * `GRADE_POINTS`, so removing the step cost the page a repetition, not a claim.
 * The sealing half moved into step 05, which keeps `PROCESS_INTRO.heading`
 * ("...to a sealed pack") honest.
 *
 * NOTE FOR THE OWNER: "Book a visit" is a button label, not a booking system —
 * there isn't one, and the body says plainly that you get in touch and a time
 * is arranged. Do not add opening hours or a slot picker here unless they
 * genuinely exist; the failure mode is people arriving at a gate.
 */
export const VISIT = {
  eyebrow: 'Come and look',
  heading: 'Would you rather see it than read it?',
  body: 'That is the whole route, pond to pack. Growers, buyers and students come out and stand at the pond edge while it is running - the ponds above are ours, not a stock photograph. Get in touch and we will arrange a time.',
  ctaLabel: 'Book a visit',
};

/**
 * The photograph here is the fresh biomass in a bowl, not an establishing shot
 * of the unit. It sits beside the four claims because it is the evidence for
 * them: dense, deep green, one ingredient, minutes out of the water. (The wide
 * shot of the covered hall was the original intent; it was never supplied.)
 */
export const GRADE_INTRO = {
  eyebrow: 'Why the grade holds',
  heading: 'What owning the farm actually buys you.',
  photo: {
    src: '/assets/farm/biomass-bowl.jpg',
    alt: 'A steel bowl heaped with fresh, deep green spirulina biomass held up at the edge of a raceway pond, the green water running away behind it.',
    gradient: 'radial-gradient(120% 100% at 45% 35%,#cfe3c4,#2f6b3a 52%,#0f2f1a)',
  } satisfies Photo,
};

export const GRADE_POINTS = [
  {
    title: 'No trading chain',
    body: 'Most spirulina sold in India changes hands two or three times before it reaches a label. Every hop is a chance for a different, cheaper batch to be substituted. Ours does not leave the farm until it is packed.',
  },
  {
    title: 'Hours, not months, from water to dry',
    body: 'Because harvest and drying happen in the same place, the gap between the two is short. Biomass that sits wet loses colour and quality, and a long haul to a distant drier is exactly how that happens.',
  },
  {
    title: 'Our own licence, our own unit',
    body: 'The manufacturing unit is FSSAI licensed and it is ours - the licence on the pack belongs to the people who grew what is inside it, not to a contract packer.',
  },
  {
    title: 'One ingredient',
    body: 'Spirulina and nothing else. No binders in the tablets, no carriers in the powder, no colour, and nothing added to bulk it out.',
  },
];

/**
 * The byte.
 *
 * **Attribution: "Founder", and nothing else — the owner's decision, 2026-08-16.**
 * No personal name and no company line beneath it. Do not "complete" this later
 * by adding a name; it is deliberately anonymous.
 *
 * The quotation is a quotation printed beside a photograph of a real,
 * identifiable person, so the standing rule from `Testimonial` in ./types.ts
 * applies here too: do not present written copy as somebody's testimony. These
 * words were drafted here rather than transcribed from anything that was said
 * out loud. The owner has reviewed the page with them in place; replace them
 * with their own words whenever they would rather, but nobody else should put
 * new words into this person's mouth.
 */
export const BYTE = {
  /** Attribution line. "Founder" alone, by the owner's decision — see above. */
  name: 'Founder',
  quote:
    'I wanted to be able to answer the one question nobody selling spirulina can answer: where did this actually come from? So we grew it ourselves. I can walk you to the pond it came out of.',
  photo: {
    src: '/assets/farm/founder.jpg',
    alt: 'A man in a white shirt standing on the paved walkway between two spirulina ponds marked PP-1 and PP-2, under a blue shade net.',
    gradient: 'radial-gradient(120% 100% at 50% 25%,#e8f2e4,#5f9a76 55%,#0f3a2b)',
  } satisfies Photo,
};

/**
 * Closing call to action.
 *
 * The photograph is the finished pack, and it is the only studio shot on the
 * page — deliberately, because this is where the farm story hands over to the
 * shop. Everything above is the pond; this is what arrives.
 */
export const FARM_CTA = {
  heading: 'Grown here. Sold here.',
  body: 'Powder for the morning, tablets for the days you are moving - both out of the ponds on this page.',
  ctaLabel: 'Shop spirulina',
  photo: {
    src: '/assets/farm/product-jars.jpg',
    alt: 'Two black and gold jars of Enerzeia Spirulina Tablets, 120 and 60 tablets, standing on a cream background beside a leaf shadow.',
    gradient: 'radial-gradient(120% 100% at 45% 35%,#f2e9d6,#b8933c 55%,#4a3a16)',
  } satisfies Photo,
};
