import { render, screen } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import TodayRecipePage from './TodayRecipePage';

vi.mock('../../api/recipes', () => ({
  getTodayRecipe: vi.fn(async () => ({
    local_date: '2026-07-30',
    timezone: 'Asia/Shanghai',
    corpus_revision: 'revision',
    recipe: {
      id: 7,
      title: '红烧鱼',
      category: 'aquatic',
      summary: '一道家常菜',
      ingredients: ['鱼', '姜'],
      dietary_warnings: [],
    },
    suggested_questions: ['需要哪些食材和用量？', '请完整说明制作步骤。', '有哪些技巧？'],
  })),
  getRecipePreferences: vi.fn(async () => ({
    dietary_restrictions: ['花生'],
    timezone: 'Asia/Shanghai',
    version: 1,
  })),
  updateRecipePreferences: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

test('renders the daily recipe, three questions, free input and preferences', async () => {
  render(<TodayRecipePage token="token" />);
  expect(await screen.findByText('今日菜谱：红烧鱼')).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: /食材|步骤|技巧/ })).toHaveLength(3);
  expect(screen.getByLabelText('烹饪问题')).toBeInTheDocument();
  expect(await screen.findByDisplayValue('花生')).toBeInTheDocument();
});
