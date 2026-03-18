import { useRealtime } from '@/hooks/useRealtime'
import {
  DashboardHeader,
  FilterBar,
  StatsGrid,
  TrafficChart,
  TopPages,
  TopReferrers,
  DevicesCard,
  BrowsersCard,
  CountriesCard,
  VisitorMapCard,
  WebVitalsCard,
  ErrorsCard,
  CampaignsCard,
  CustomEventsCard,
  OutboundLinksCard,
  AdSpendCard,
} from './index'

export function Dashboard() {
  useRealtime()

  return (
    <div className="p-6 space-y-6 overflow-y-auto h-full">
      <DashboardHeader />
      <FilterBar />
      <StatsGrid />
      <TrafficChart />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <TopPages />
        <TopReferrers />
      </div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <DevicesCard />
        <BrowsersCard />
        <CountriesCard />
      </div>
      <VisitorMapCard />
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <WebVitalsCard />
        <ErrorsCard />
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <CampaignsCard />
        <CustomEventsCard />
        <OutboundLinksCard />
      </div>
      <AdSpendCard />
    </div>
  )
}
