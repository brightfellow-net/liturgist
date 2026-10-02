// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { createBrowserRouter } from "react-router";
import { AppLayout } from "./routes/AppLayout";
import { HomePage } from "./routes/HomePage";
import { LoginPage } from "./routes/LoginPage";
import { NotFoundPage } from "./routes/NotFoundPage";
import { PrivacyPage } from "./routes/PrivacyPage";
import { SetupPage } from "./routes/SetupPage";
import { paths } from "./routes/paths";

export const router = createBrowserRouter([
  { path: paths.login, element: <LoginPage /> },
  { path: paths.setup, element: <SetupPage /> },
  { path: paths.privacy, element: <PrivacyPage /> },
  {
    element: <AppLayout />,
    children: [{ path: paths.home, element: <HomePage /> }],
  },
  { path: "*", element: <NotFoundPage /> },
]);
