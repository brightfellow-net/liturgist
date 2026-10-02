// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { createBrowserRouter } from "react-router";
import { AppLayout } from "./routes/AppLayout";
import { HomePage } from "./routes/HomePage";
import { InvitePage } from "./routes/InvitePage";
import { LoginPage } from "./routes/LoginPage";
import { NotFoundPage } from "./routes/NotFoundPage";
import { PrivacyPage } from "./routes/PrivacyPage";
import { ProfilePage } from "./routes/ProfilePage";
import { ResetPage } from "./routes/ResetPage";
import { SetupPage } from "./routes/SetupPage";
import { paths } from "./routes/paths";

export const router = createBrowserRouter([
  { path: paths.login, element: <LoginPage /> },
  { path: paths.setup, element: <SetupPage /> },
  { path: paths.invite, element: <InvitePage /> },
  { path: paths.reset, element: <ResetPage /> },
  { path: paths.privacy, element: <PrivacyPage /> },
  {
    element: <AppLayout />,
    children: [
      { path: paths.home, element: <HomePage /> },
      { path: paths.profile, element: <ProfilePage /> },
    ],
  },
  { path: "*", element: <NotFoundPage /> },
]);
