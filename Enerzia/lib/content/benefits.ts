/**
 * "Benefits" page copy, lifted from the Benefits design in the Claude Design
 * project (Benefits.dc.html, imported 2026-08-27).
 *
 * ---------------------------------------------------------------------------
 * WHY THIS PAGE IS WRITTEN THE WAY IT IS
 *
 * This is the page a supplement brand normally lies on. The design's own
 * headline calls that out - "It's a food, not a promise" - and the structure
 * enforces it: every reason card states a NUTRITIONAL FACT about the food and
 * then, in its own footer, says how far the research actually goes. The
 * `WHAT WE DON'T CLAIM` panel is not decoration; it is the reason the rest of
 * the page is credible.
 *
 * So the editing rule here is stricter than elsewhere in the repo:
 *
 *   - A composition fact (protein %, iron, pigment) may be stated plainly.
 *     It is measurable and it is what the pack's own lab report carries.
 *   - A physiological effect may NOT be stated as an outcome. It goes in
 *     `evidence`, phrased as what the trials do and do not settle.
 *   - Nothing here may promise treatment, cure, prevention, weight loss or
 *     detoxification. Those four are listed in `LIMITS` as things we refuse to
 *     claim - do not reintroduce them anywhere above.
 *
 * `NUTRIENTS` is labelled as TYPICAL PUBLISHED VALUES for dried spirulina, not
 * as our assay, and the panel says so twice - in `NUTRITION_INTRO.body` and in
 * `NUTRITION_FOOTNOTE`. Keep both. The moment this table reads as *our* numbers
 * it becomes a nutrition declaration, and a nutrition declaration that differs
 * from the printed pack is an FSSAI problem, not a copy problem.
 * ---------------------------------------------------------------------------
 *
 * ONE DELIBERATE DEPARTURE FROM THE DESIGN AS DRAWN
 *
 * The design's closing CTA reads "Shop spirulina - from Rs 200". No price is
 * quoted here. Prices are the manager's, they live in the catalogue (Mongo)
 * and the storefront cannot know them statically - and the homepage's own
 * hardcoded "from Rs 400" already contradicts the design. A benefits page does
 * not need a number it cannot keep true.
 *
 * ---------------------------------------------------------------------------
 * IMAGES ON THIS PAGE - READ BEFORE CHANGING ANY OF THEM
 *
 * All eight are GENERATED still-lifes supplied by the owner on 2026-08-27, one
 * per slot in the design. Originals are kept at `assets/benefits-originals/`.
 *
 * Six of them - the pulses, the slate and lemon, the pigment in water, the
 * powder on paper, the dried flakes - carry no branding at all and sit
 * comfortably inside the standing rule in ./home.ts: images that illustrate
 * *use* or the food itself may be generated, because none of them asserts
 * anything a buyer is being asked to trust.
 *
 * THE OTHER THREE SHOW A PACK, AND THAT IS THE OWNER'S EXPLICIT DECISION.
 * `spoon-and-jar`, `kitchen-counter-band` and `tablets-in-palm` show an
 * Enerzeia pack whose label does not match the one in
 * `/assets/farm/product-jars.jpg`: a dark jar with a serif ENERZEIA wordmark
 * under a sunrise-over-hills mark, where the real pack is green and gold and
 * the real mark is the swirl in `BrandLockup`. (`powder-on-paper` carries the
 * same invented mark on a wooden scoop.) This was put to the owner with the
 * conflicting ./home.ts clause quoted - "a generator invents a label, and an
 * invented Enerzeia jar is worse than showing no jar at all" - and they
 * supplied these images afterwards, for use.
 *
 * So: do NOT quietly swap these out as a bug. It is a live inconsistency the
 * owner has accepted, and reversing it is their call, not a maintainer's. If
 * they ever want it resolved, the fix is a photograph or a regeneration using
 * the real green-and-gold label, dropped in under the same filenames-with-new-
 * names (see the cache note in `components/Photo.tsx`).
 * ---------------------------------------------------------------------------
 */

import type { Photo } from './types';

export const BENEFITS_HERO = {
  eyebrow: 'Benefits, honestly',
  /** Rendered as three lines; the breaks are intentional typesetting. */
  headingLines: ["It's a food, not a", "promise. Here's", 'what it adds.'],
  body: 'Spirulina is a blue-green algae eaten as food for centuries, and one of the densest edible sources of plant protein, iron and carotenoids we know of. Below is what a spoonful contributes - and where the honest limits of the evidence sit.',
  ctaLabel: 'See the numbers',
  quietCtaLabel: "What we don't claim",
  photo: {
    src: '/assets/benefits/spoon-and-jar.jpg',
    alt: 'A worn silver spoon heaped with deep blue-green spirulina powder resting on cream linen, a little powder spilled beside it, with a dark Enerzeia Spirulina powder jar standing behind on a weathered wooden table.',
    gradient: 'radial-gradient(125% 100% at 25% 15%,#cfe9d5,#5f9a76 55%,#123f2d)',
  } satisfies Photo,
};

/** One of the four figures in the strip under the hero. */
export interface Topline {
  /** The large numeral. Kept short - it sets the column width. */
  big: string;
  title: string;
  body: string;
}

/**
 * Every figure here is already claimed elsewhere in the repo, or is arithmetic
 * on one that is:
 *   62%+ protein     - ./home.ts HERO.badgeBig and HERO_CHIPS
 *   one ingredient   - ./farm.ts GRADE_POINTS, and the tablets product copy
 *   20 servings      - 100 g pack / 5 g serving
 *   ~20 kcal         - the 5 g serving row in NUTRIENTS
 * Do not add a fifth without a source in the same shape.
 */
export const TOPLINE: Topline[] = [
  {
    big: '62%+',
    title: 'Protein by dry weight',
    body: 'More than three times the protein density of most cooked pulses, with all nine essential amino acids present.',
  },
  {
    big: '1',
    title: 'One ingredient, nothing added',
    body: 'No binders, fillers, sweeteners or colours - in the powder or the pressed tablets.',
  },
  {
    big: '20',
    title: 'Servings in a 100 g pack',
    body: 'A level teaspoon a day. Roughly a month of habit per pack.',
  },
  {
    big: '~20',
    title: 'Kilocalories per serving',
    body: 'Nutrient density without meaningful calories, so it fits any way of eating.',
  },
];

export const NUTRITION_INTRO = {
  eyebrow: 'Nutrition',
  heading: 'What one teaspoon carries',
  body: "Typical published values for dried spirulina at a 5 g serving - about a level teaspoon of powder, or ten of our 500 mg tablets. Your pack's own lab report is the number that counts, and it's printed on the label.",
};

/** One row of the nutrition panel. */
export interface Nutrient {
  name: string;
  /** A range, not a single figure - see the header note. */
  value: string;
  note: string;
}

export const NUTRIENTS: Nutrient[] = [
  { name: 'Protein', value: '3 - 3.5 g', note: 'Complete amino acid profile; ~60-70% of dry weight' },
  { name: 'Iron', value: '1.4 - 2.5 mg', note: 'Non-haem; absorbs better with vitamin C' },
  { name: 'Phycocyanin', value: '0.5 - 0.9 g', note: 'The blue pigment-protein unique to spirulina' },
  { name: 'Beta-carotene', value: '1 - 2 mg', note: 'Pro-vitamin A carotenoid' },
  { name: 'Riboflavin (B2)', value: '0.15 - 0.2 mg', note: 'Plus thiamine and niacin in smaller amounts' },
  { name: 'Chlorophyll', value: '~50 mg', note: 'Green pigment; the reason the glass stains' },
  { name: 'Energy', value: '~20 kcal', note: 'Per 5 g serving' },
];

/** The second of the two disclaimers this table must always carry. */
export const NUTRITION_FOOTNOTE =
  'Values are typical ranges for sun-dried spirulina and vary by harvest. Not a substitute for the declared nutrition panel on the pack.';

export const REASONS_INTRO = {
  eyebrow: 'Why people take it',
  heading: 'Six reasons, and what sits behind each',
  body: "Each one below is a nutritional fact about the food, followed by what research has and hasn't settled.",
};

/**
 * One reason, with its own evidence footer.
 *
 * `evidence` is REQUIRED, and that is the page's central rule made into a
 * compile error: a benefit card without a line saying how far the research
 * goes is the exact shape of the marketing this page exists to refuse. A claim
 * nobody can qualify does not belong in this list.
 */
export interface Reason {
  /** Short uppercase chip, e.g. "PROTEIN". */
  tag: string;
  title: string;
  body: string;
  /** How far the research actually goes. Never an outcome promise. */
  evidence: string;
  /**
   * Required, like `ProcessStep.photo` on /farm and for the same reason: the
   * six sit in one grid, so a card without a picture reads as a missing
   * photograph rather than as a design. If a seventh reason is ever added it
   * needs its own image, not a reused one.
   */
  photo: Photo;
}

/**
 * The six reasons, each with its own photograph and its own evidence footer.
 *
 * Each `gradient` below is the one the design specified for that card, and
 * they are tuned to their photographs - blue-green for the pigment, warm gold
 * for the B-vitamins, olive for the flakes. They are the fallback if a file is
 * missing, so keep them matched when a photograph is replaced.
 */
export const REASONS: Reason[] = [
  {
    tag: 'PROTEIN',
    title: 'A complete plant protein',
    body: 'Sixty to seventy per cent protein by dry weight, containing all nine essential amino acids - rare for a plant. Useful padding for vegetarian and vegan diets that lean on pulses and grains.',
    evidence: 'Settled. This is straightforward composition, measurable in any lab.',
    photo: {
      src: '/assets/benefits/powder-with-pulses.jpg',
      alt: 'Overhead view of a heap of deep green spirulina powder on a cream surface beside three small stone bowls holding red lentils, chickpeas and rice.',
      gradient: 'radial-gradient(120% 100% at 30% 25%,#cfe9d5,#5f9a76 55%,#123f2d)',
    },
  },
  {
    tag: 'IRON',
    title: 'Iron in a plant form',
    body: 'One of the denser plant sources of iron, alongside riboflavin - the pair most often short in diets without much meat. Take it with something citrus to help absorption.',
    evidence:
      'Well established as a source. Whether it corrects anaemia depends on the deficiency and needs a doctor.',
    photo: {
      src: '/assets/benefits/spoon-on-slate-lemon.jpg',
      alt: 'A brass spoon heaped with dark green spirulina powder resting on a round slate plate in low sunlight, a cut lemon half beside it.',
      gradient: 'radial-gradient(120% 100% at 70% 30%,#e3ddc9,#8b7a4e 55%,#3d3418)',
    },
  },
  {
    tag: 'PHYCOCYANIN',
    title: 'The blue pigment',
    body: 'Phycocyanin is the protein-pigment that makes spirulina blue-green, and the compound most studied for antioxidant activity in cell and animal models.',
    evidence:
      'Promising in the lab; human trials are small and short. We will not translate them into promises.',
    photo: {
      src: '/assets/benefits/pigment-in-water.jpg',
      alt: 'A macro photograph of blue and green pigment dispersing through water in fine billowing threads against a pale background.',
      gradient: 'radial-gradient(120% 100% at 45% 25%,#cfe4ea,#4e8ea0 55%,#123845)',
    },
  },
  {
    tag: 'B-VITAMINS',
    title: 'Everyday B-vitamins',
    body: 'Riboflavin, thiamine and niacin in modest but real amounts - the vitamins involved in turning food into usable energy.',
    evidence:
      'Composition is clear. Note that spirulina B12 is largely a pseudo-form and should not be counted on.',
    photo: {
      src: '/assets/benefits/powder-on-paper.jpg',
      alt: 'Deep green spirulina powder swept in a wide arc across textured handmade paper, with a wooden measuring scoop of powder resting at the end of the sweep.',
      gradient: 'radial-gradient(120% 100% at 60% 70%,#f0e6cd,#c1a45f 55%,#5a4a1d)',
    },
  },
  {
    tag: 'GLA',
    title: 'A rare fatty acid',
    body: 'Spirulina carries gamma-linolenic acid, an omega-6 fatty acid otherwise mostly found in evening primrose and borage oil.',
    evidence:
      'Present in meaningful amounts; the health significance at food-level doses is not established.',
    photo: {
      src: '/assets/benefits/dried-flakes.jpg',
      alt: 'A woven bamboo winnowing basket heaped with dark blue-green dried spirulina flakes, lit from the side on a linen cloth.',
      gradient: 'radial-gradient(120% 100% at 35% 65%,#e8efd6,#93ac67 55%,#3c4a22)',
    },
  },
  {
    tag: 'HABIT',
    title: "It's easy to keep up",
    body: 'One teaspoon in a smoothie or ten tablets with water. The nutrition only counts if you actually take it - which is why we sell both forms.',
    evidence: 'Not a nutrition claim, just the thing that decides whether any of the above matters.',
    photo: {
      src: '/assets/benefits/tablets-in-palm.jpg',
      alt: 'An open palm holding about ten small dark green spirulina tablets, with a glass of water and a bottle of Enerzeia Spirulina Tablets on the counter behind.',
      gradient: 'radial-gradient(120% 100% at 55% 35%,#dfeed3,#7fa85c 55%,#2d5327)',
    },
  },
];

export const SUITS_INTRO = {
  heading: 'Who tends to feel the difference',
  body: "Spirulina helps most where a diet is already short of something. If you're eating well and varied, treat it as insurance rather than a fix.",
  /**
   * The wide band across the top of this section. Composed for 21:8: the left
   * third is bare sunlit wall, so the frame can be cropped hard from either
   * side at narrow widths without losing the subject.
   */
  photo: {
    src: '/assets/benefits/kitchen-counter-band.jpg',
    alt: 'A woman at a sunlit kitchen counter stirring a glass of deep green spirulina drink with a long spoon, an open Enerzeia Spirulina jar and a bowl of lemons and bananas beside her.',
    gradient: 'radial-gradient(120% 100% at 70% 40%,#f2e9d6,#c7b28a 55%,#4a3a16)',
  } satisfies Photo,
};

export interface Suit {
  /** Two-digit ordinal shown in the ring above the title. */
  n: string;
  title: string;
  body: string;
}

export const SUITS: Suit[] = [
  {
    n: '01',
    title: 'Vegetarian and vegan eaters',
    body: 'Where protein variety and iron are the two usual gaps, spirulina covers both from one ingredient.',
  },
  {
    n: '02',
    title: 'People low on iron',
    body: 'A dense plant source to eat alongside, not instead of, whatever your doctor has advised.',
  },
  {
    n: '03',
    title: 'Anyone training hard',
    body: 'Extra protein without extra bulk, plus the B-vitamins that energy metabolism runs on.',
  },
  {
    n: '04',
    title: 'Busy, irregular eaters',
    body: 'Skipped meals and canteen food. A teaspoon is a floor under a diet you cannot always control.',
  },
];

export const DOSING_INTRO = {
  eyebrow: 'How much, and when',
  heading: 'Start small, keep it daily',
  footnote: 'Consistency beats quantity. A daily teaspoon does more than an occasional tablespoon.',
};

/**
 * The dosing ladder.
 *
 * Kept in step with ./home.ts STARTING_OUT and HOME_FAQS[0], which already
 * tell a buyer to begin low for the first week and settle at 3-5 g. If one of
 * the three changes, change all three - a storefront that gives two different
 * daily amounts on two pages is worse than one that gives none.
 */
export const DOSING: { k: string; v: string }[] = [
  { k: 'Week 1', v: '1-2 g a day - half a teaspoon of powder, or 3-4 tablets - taken with food.' },
  { k: 'Onward', v: '3-5 g a day for most adults. Split it across two times if you prefer.' },
  { k: 'When', v: 'Any time with a meal or a drink. Some find it too energising late in the evening.' },
  { k: 'Storage', v: 'Sealed, dry, out of sunlight. Spirulina fades and loses pigment in the light.' },
];

export const COMPARE_HEADING = 'Powder or tablets?';

export const COMPARE: { label: string; text: string }[] = [
  {
    label: 'Powder',
    text: 'Cheaper per gram, easy to scale up or down, and it blends into food. Tastes green - best with fruit.',
  },
  {
    label: 'Tablets',
    text: 'No taste, no mixing, no green glass. Portable, and the honest answer for anyone who dislikes the flavour.',
  },
  {
    label: 'Both',
    text: 'Powder at home, tablets in the bag. That is the pattern most repeat buyers land on.',
  },
];

export const COMPARE_CTA_LABEL = 'See the packs';

export const LIMITS_HEADING = "What we don't claim";

/**
 * The four refusals.
 *
 * This list is the page's licence to make every claim above it, so it is the
 * last thing that should be trimmed for space. Each entry names a claim the
 * supplement trade routinely makes and we do not.
 */
export const LIMITS: string[] = [
  'That it treats, cures or prevents any illness.',
  'That it causes weight loss, or replaces a meal.',
  'That it detoxes anything - your liver and kidneys do that.',
  'That it supplies usable vitamin B12. Take a proper B12 supplement if you need one.',
];

/**
 * Mirrors the caution in Terms clause 4 and ./home.ts STARTING_OUT.caution,
 * with the two additions the design makes explicit - autoimmune and metabolic
 * conditions, phenylketonuria. Keep all three in step.
 */
export const LIMITS_DISCLAIMER =
  'Spirulina is a food supplement. It is not intended to diagnose, treat, cure or prevent any condition. If you are pregnant, nursing, on medication, or have an autoimmune or metabolic condition (including phenylketonuria), speak to your doctor first.';

export interface BenefitFaq {
  q: string;
  a: string;
}

export const FAQ_HEADING = 'Questions about the benefits';

/**
 * A different set from the homepage's four and from the eleven on /faq: these
 * are the benefit-shaped questions, and two of them exist specifically to say
 * no. Do not soften "Does it help with weight loss?" - the honest answer is
 * the reason anyone believes the rest of the page.
 */
export const BENEFIT_FAQS: BenefitFaq[] = [
  {
    q: 'Is spirulina better than a multivitamin?',
    a: 'They are different things. A multivitamin is isolated nutrients at fixed doses; spirulina is a whole food that happens to be nutrient-dense, so it also brings protein, pigments and fibre-like material. If you have a diagnosed deficiency, a targeted supplement your doctor prescribes will move a blood marker faster.',
  },
  {
    q: 'How long before I notice anything?',
    a: 'People most often report a difference in energy and digestion after two to four weeks of daily use. Nutritional intake changes from day one; how you feel depends on what your diet was short of to begin with. If nothing changes after a month, it may simply be filling a gap you did not have.',
  },
  {
    q: 'Does it help with weight loss?',
    a: 'Not by itself. It is protein-dense and low in calories, so it can make a meal or shake more filling for fewer calories - that is the whole mechanism. Anyone promising fat loss from a green powder is selling you something.',
  },
  {
    q: 'Is the iron in spirulina actually absorbed?',
    a: 'It is non-haem iron, the same kind found in lentils and greens, so absorption is lower than from meat and improves markedly with vitamin C. Take it with citrus, amla, tomato or bell pepper rather than with tea or coffee, which inhibit uptake.',
  },
  {
    q: 'Chlorella, moringa or spirulina?',
    a: 'Spirulina leads on protein and phycocyanin; chlorella has a tougher cell wall and is usually taken for its binding properties; moringa is a leaf, higher in calcium and vitamin C but much lower in protein. They are complements, not competitors - no need to take all three.',
  },
];

export const BENEFITS_CTA = {
  heading: 'Try it for a month and judge for yourself.',
  body: 'A 100 g pack is about twenty servings - enough to know whether the habit sticks before you commit to more.',
  /** No price - see the departures note at the top of this file. */
  ctaLabel: 'Shop spirulina',
};
