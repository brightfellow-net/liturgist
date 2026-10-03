// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { createBrowserRouter } from "react-router";
import { AppLayout } from "./routes/AppLayout";
import { HomePage } from "./routes/HomePage";
import { InvitePage } from "./routes/InvitePage";
import { LoginPage } from "./routes/LoginPage";
import { LibraryLayout } from "./routes/library/LibraryLayout";
import { LibraryPage } from "./routes/library/LibraryPage";
import { ReadingFormPage } from "./routes/library/ReadingFormPage";
import { ReadingPage } from "./routes/library/ReadingPage";
import { ReadingsPage } from "./routes/library/ReadingsPage";
import { SongFormPage } from "./routes/library/SongFormPage";
import { SongPage } from "./routes/library/SongPage";
import { NotFoundPage } from "./routes/NotFoundPage";
import { PrivacyPage } from "./routes/PrivacyPage";
import { ProfilePage } from "./routes/ProfilePage";
import { ResetPage } from "./routes/ResetPage";
import { ChurchSettingsPage } from "./routes/settings/ChurchSettingsPage";
import { MembersPage } from "./routes/settings/MembersPage";
import { RolesPage } from "./routes/settings/RolesPage";
import { SettingsLayout } from "./routes/settings/SettingsLayout";
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
      {
        element: <LibraryLayout />,
        children: [
          { path: paths.library, element: <LibraryPage /> },
          { path: paths.readings, element: <ReadingsPage /> },
        ],
      },
      { path: paths.readingNew, element: <ReadingFormPage /> },
      { path: paths.reading, element: <ReadingPage /> },
      { path: paths.songNew, element: <SongFormPage /> },
      { path: paths.song, element: <SongPage /> },
      { path: paths.songEdit, element: <SongFormPage /> },
      { path: paths.profile, element: <ProfilePage /> },
      {
        element: <SettingsLayout />,
        children: [
          { path: paths.churchSettings, element: <ChurchSettingsPage /> },
          { path: paths.members, element: <MembersPage /> },
          { path: paths.roles, element: <RolesPage /> },
        ],
      },
    ],
  },
  { path: "*", element: <NotFoundPage /> },
]);
