export function wallAlertCount(heights: number[], available: number, gap: number, moreHeight: number): number {
  const total = heights.reduce((sum, height) => sum + height, 0) + Math.max(0, heights.length - 1) * gap;
  if (total <= available) return heights.length;
  let used = moreHeight;
  let count = 0;
  for (const height of heights) {
    if (used + height + gap > available) break;
    used += height + gap;
    count++;
  }
  return count;
}
