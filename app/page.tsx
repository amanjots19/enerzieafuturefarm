import { redirect } from 'next/navigation';

/* The marketing landing page still lives as the static index.html at the repo
   root and has not been migrated to the App Router yet, so `/` forwards to the
   shop for now. Replace this with the ported landing page. */
export default function Home() {
  redirect('/shop');
}
