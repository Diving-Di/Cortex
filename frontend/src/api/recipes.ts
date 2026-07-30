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
