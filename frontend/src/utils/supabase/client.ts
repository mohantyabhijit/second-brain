import { createBrowserClient } from "@supabase/ssr";

const supabaseUrl = process.env.NEXT_PUBLIC_SUPABASE_URL;
const supabaseKey = process.env.NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY;

export const isSupabaseConfigured = Boolean(supabaseUrl && supabaseKey);

const createClient = () => {
  if (!supabaseUrl || !supabaseKey) {
    throw new Error("NEXT_PUBLIC_SUPABASE_URL and NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY are required for browser Supabase auth.");
  }
  return createBrowserClient(supabaseUrl, supabaseKey);
};

export const tryCreateClient = () => {
  if (!isSupabaseConfigured) {
    return null;
  }
  return createClient();
};
