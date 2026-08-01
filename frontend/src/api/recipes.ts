import { authHeaders, http } from './http';

export interface TodayRecipe {
  local_date: string;
  timezone: string;
  corpus_revision: string;
  recipe: {
    id: number;
    title: string;
    category: string;
    summary: string;
    difficulty?: string;
    calories_text?: string;
    ingredients: string[];
    dietary_warnings: string[];
  };
  suggested_questions: string[];
}

export interface RecipePreferences {
  dietary_restrictions: string[];
  timezone: string;
  version: number;
  marketplace_personalization: boolean;
}

export interface RecipeConversation {
  id: number;
  title: string;
  source_scope: 'recipe';
  created_at: string;
  updated_at: string;
  version: number;
  message_count: number;
  total_tokens: number;
  messages?: Array<{
    id: number;
    role: 'user' | 'assistant';
    content: string;
    created_at: string;
  }>;
}

export async function getTodayRecipe(token: string) {
  const response = await http.get<TodayRecipe>('/api/v1/recipes/today', {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function getRecipePreferences(token: string) {
  const response = await http.get<RecipePreferences>('/api/v1/settings/preferences', {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function updateRecipePreferences(token: string, value: RecipePreferences) {
  const response = await http.put<RecipePreferences>('/api/v1/settings/preferences', value, {
    headers: authHeaders(token),
  });
  return response.data;
}

export async function listRecipeConversations(token: string) {
  const response = await http.get<{ items: RecipeConversation[]; total: number }>(
    '/api/v1/conversations',
    {
      headers: authHeaders(token),
      params: { source_scope: 'recipe', limit: 100 },
    },
  );
  return response.data?.items || [];
}

export async function getRecipeConversation(token: string, id: number) {
  const response = await http.get<RecipeConversation>(`/api/v1/conversations/${id}`, {
    headers: authHeaders(token),
  });
  return response.data;
}
