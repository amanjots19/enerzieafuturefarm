import { redirect } from 'next/navigation';

// Visiting the root always goes to /login. The login page redirects to
// /products if a valid token is already in sessionStorage.
export default function Home() {
  redirect('/login');
}
