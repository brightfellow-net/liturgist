// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { createBrowserRouter } from "react-router";
import { AppLayout } from "./routes/AppLayout";
import { HomePage } from "./routes/HomePage";
import { InvitePage } from "./routes/InvitePage";
import { LoginPage } from "./routes/LoginPage";
import { ImportPage } from "./routes/library/ImportPage";
import { ImportReviewPage } from "./routes/library/ImportReviewPage";
import { LibraryLayout } from "./routes/library/LibraryLayout";
import { LibraryPage } from "./routes/library/LibraryPage";
import { ReadingFormPage } from "./routes/library/ReadingFormPage";
import { ReadingPage } from "./routes/library/ReadingPage";
import { ReadingsPage } from "./routes/library/ReadingsPage";
import { SongFormPage } from "./routes/library/SongFormPage";
import { SongPage } from "./routes/library/SongPage";
import { LiturgiesPage } from "./routes/liturgy/LiturgiesPage";
import { LiturgyPage } from "./routes/liturgy/LiturgyPage";
import { NewLiturgyPage } from "./routes/liturgy/NewLiturgyPage";
import { PreparePage } from "./routes/liturgy/PreparePage";
import { NotFoundPage } from "./routes/NotFoundPage";
import { NameListPage } from "./routes/planning/NameListPage";
import { PlanningLayout } from "./routes/planning/PlanningLayout";
import { ServiceFormPage } from "./routes/planning/ServiceFormPage";
import { ServicesPage } from "./routes/planning/ServicesPage";
import { TemplateFormPage } from "./routes/planning/TemplateFormPage";
import { TemplatesPage } from "./routes/planning/TemplatesPage";
import { dutyList, partList } from "./lib/planning";
import { PrivacyPage } from "./routes/PrivacyPage";
import { ProfilePage } from "./routes/ProfilePage";
import { PublishedListPage } from "./routes/published/PublishedListPage";
import { PublishedPage } from "./routes/published/PublishedPage";
import { PrintPage } from "./routes/published/PrintPage";
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
      { path: paths.import, element: <ImportPage /> },
      { path: paths.importReview, element: <ImportReviewPage /> },
      { path: paths.songNew, element: <SongFormPage /> },
      { path: paths.song, element: <SongPage /> },
      { path: paths.songEdit, element: <SongFormPage /> },
      {
        element: <PlanningLayout />,
        children: [
          { path: paths.planning, element: <LiturgiesPage /> },
          { path: paths.templates, element: <TemplatesPage /> },
          { path: paths.services, element: <ServicesPage /> },
          { path: paths.duties, element: <NameListPage list={dutyList} /> },
          { path: paths.singingParts, element: <NameListPage list={partList} /> },
        ],
      },
      { path: paths.liturgyPrepare, element: <PreparePage /> },
      { path: paths.liturgyNew, element: <NewLiturgyPage /> },
      { path: paths.liturgy, element: <LiturgyPage /> },
      { path: paths.templateNew, element: <TemplateFormPage /> },
      { path: paths.template, element: <TemplateFormPage /> },
      { path: paths.serviceNew, element: <ServiceFormPage /> },
      { path: paths.service, element: <ServiceFormPage /> },
      { path: paths.published, element: <PublishedListPage /> },
      { path: paths.publishedView, element: <PublishedPage /> },
      { path: paths.publishedPrint, element: <PrintPage /> },
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
