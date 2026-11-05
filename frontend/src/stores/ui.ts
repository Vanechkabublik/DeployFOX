import { create } from "zustand";
type UI = {
  page: string;
  mobile: boolean;
  setPage: (page: string) => void;
  toggleMobile: () => void;
};
export const useUI = create<UI>((set) => ({
  page: "Overview",
  mobile: false,
  setPage: (page) => set({ page, mobile: false }),
  toggleMobile: () => set((state) => ({ mobile: !state.mobile })),
}));
