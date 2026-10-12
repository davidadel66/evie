import { Banner } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

export const Notice = () => (
  <AppFrame>
    <Banner message="Spending could not be loaded. Previous records remain visible." onDismiss={() => {}} />
  </AppFrame>
);

export const Problem = () => (
  <AppFrame>
    <Banner message="Citibank Online could not be refreshed: the bank asked you to sign in again." onDismiss={() => {}} />
  </AppFrame>
);

export const LongMessage = () => (
  <AppFrame>
    <Banner
    message="The finance-review run is waiting for your answer. Open the Finance workspace to accept, reject or change each proposed category; nothing is written until you do."
    onDismiss={() => {}}
  />
  </AppFrame>
);
