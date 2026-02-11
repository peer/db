<script setup lang="ts">
import TableOfContents from "@/components/TableOfContents.vue"
import NavBar from "@/partials/NavBar.vue"

const sections = [
  { id: "personal-info", label: "Personal information" },
  { id: "contact", label: "Contact details" },
  { id: "address", label: "Address" },
  { id: "education", label: "Education" },
  { id: "employment", label: "Employment history" },
  { id: "skills", label: "Skills & expertise" },
  { id: "references", label: "References" },
  { id: "documents", label: "Documents & uploads" },
  { id: "preferences", label: "Preferences" },
  { id: "review", label: "Review & submit" },
]
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="mt-12 flex w-full sm:mt-[4.5rem]">
    <!-- Table of contents sidebar. -->
    <div class="hidden w-56 shrink-0 lg:block">
      <TableOfContents :sections="sections" />
    </div>

    <!-- Main content column. -->
    <div class="min-w-0 flex-1 p-4 sm:p-8">
      <h1 class="mb-8 text-3xl font-bold">Demo form</h1>

      <section id="personal-info" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Personal information</h2>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">First name</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="John" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Last name</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Doe" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Date of birth</label>
            <input type="date" class="w-full rounded-sm border border-gray-300 px-3 py-2" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Nationality</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Slovenian" />
          </div>
        </div>
        <p class="mt-4 text-sm text-gray-500">
          Please provide your legal name as it appears on official documents. This information is used for identification purposes and will be kept confidential in
          accordance with our privacy policy.
        </p>
      </section>

      <section id="contact" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Contact details</h2>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Email</label>
            <input type="email" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="john@example.com" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Phone</label>
            <input type="tel" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="+386 1 234 5678" />
          </div>
          <div class="sm:col-span-2">
            <label class="mb-1 block text-sm font-medium text-gray-700">Alternative email</label>
            <input type="email" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="john.backup@example.com" />
          </div>
        </div>
        <p class="mt-4 text-sm text-gray-500">
          We will use your primary email for all communications. Your phone number may be used for urgent matters or two-factor authentication.
        </p>
      </section>

      <section id="address" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Address</h2>
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div class="sm:col-span-2">
            <label class="mb-1 block text-sm font-medium text-gray-700">Street address</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Presernov trg 1" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">City</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Ljubljana" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Postal code</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="1000" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Country</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Slovenia" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Region</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Osrednjeslovenska" />
          </div>
        </div>
      </section>

      <section id="education" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Education</h2>
        <div class="space-y-6">
          <div class="rounded-sm border border-gray-200 p-4">
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div class="sm:col-span-2">
                <label class="mb-1 block text-sm font-medium text-gray-700">Institution</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="University of Ljubljana" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Degree</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Bachelor of Science" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Field of study</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Computer Science" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Start year</label>
                <input type="number" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="2018" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">End year</label>
                <input type="number" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="2022" />
              </div>
            </div>
          </div>
          <p class="text-sm text-gray-500"
            >List your educational background starting with the most recent. Include any certifications or professional development courses.</p
          >
        </div>
      </section>

      <section id="employment" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Employment history</h2>
        <div class="space-y-6">
          <div class="rounded-sm border border-gray-200 p-4">
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div class="sm:col-span-2">
                <label class="mb-1 block text-sm font-medium text-gray-700">Company</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Acme Corp" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Position</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Software Engineer" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Location</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Ljubljana, Slovenia" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Start date</label>
                <input type="date" class="w-full rounded-sm border border-gray-300 px-3 py-2" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">End date</label>
                <input type="date" class="w-full rounded-sm border border-gray-300 px-3 py-2" />
              </div>
              <div class="sm:col-span-2">
                <label class="mb-1 block text-sm font-medium text-gray-700">Description</label>
                <textarea class="w-full rounded-sm border border-gray-300 px-3 py-2" rows="3" placeholder="Describe your responsibilities and achievements..."></textarea>
              </div>
            </div>
          </div>
          <p class="text-sm text-gray-500">
            Include all relevant work experience. Provide details about your role, responsibilities, and notable achievements at each position.
          </p>
        </div>
      </section>

      <section id="skills" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Skills & expertise</h2>
        <div class="space-y-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Technical skills</label>
            <textarea class="w-full rounded-sm border border-gray-300 px-3 py-2" rows="3" placeholder="Go, TypeScript, Vue.js, PostgreSQL, Elasticsearch..."></textarea>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Languages</label>
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <input type="text" class="rounded-sm border border-gray-300 px-3 py-2" placeholder="Slovenian (native)" />
              <input type="text" class="rounded-sm border border-gray-300 px-3 py-2" placeholder="English (fluent)" />
              <input type="text" class="rounded-sm border border-gray-300 px-3 py-2" placeholder="German (basic)" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Certifications</label>
            <textarea class="w-full rounded-sm border border-gray-300 px-3 py-2" rows="2" placeholder="List any professional certifications..."></textarea>
          </div>
        </div>
      </section>

      <section id="references" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">References</h2>
        <div class="space-y-6">
          <div class="rounded-sm border border-gray-200 p-4">
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Name</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Jane Smith" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Relationship</label>
                <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Former manager" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Email</label>
                <input type="email" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="jane@example.com" />
              </div>
              <div>
                <label class="mb-1 block text-sm font-medium text-gray-700">Phone</label>
                <input type="tel" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="+386 1 234 5678" />
              </div>
            </div>
          </div>
          <p class="text-sm text-gray-500">Provide at least two professional references. We will contact them only after notifying you.</p>
        </div>
      </section>

      <section id="documents" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Documents & uploads</h2>
        <div class="space-y-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Resume / CV</label>
            <div class="rounded-sm border-2 border-dashed border-gray-300 p-6 text-center">
              <p class="text-sm text-gray-500">Drag and drop your file here, or click to browse</p>
              <p class="mt-1 text-xs text-gray-400">PDF, DOC, or DOCX up to 10MB</p>
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Cover letter</label>
            <div class="rounded-sm border-2 border-dashed border-gray-300 p-6 text-center">
              <p class="text-sm text-gray-500">Drag and drop your file here, or click to browse</p>
              <p class="mt-1 text-xs text-gray-400">PDF, DOC, or DOCX up to 10MB</p>
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Additional documents</label>
            <textarea
              class="w-full rounded-sm border border-gray-300 px-3 py-2"
              rows="2"
              placeholder="Describe any additional documents you would like to attach..."
            ></textarea>
          </div>
        </div>
      </section>

      <section id="preferences" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Preferences</h2>
        <div class="space-y-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Preferred start date</label>
            <input type="date" class="rounded-sm border border-gray-300 px-3 py-2" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Work arrangement</label>
            <div class="flex gap-4">
              <label class="flex items-center gap-2 text-sm">
                <input type="radio" name="work-arrangement" value="office" />
                Office
              </label>
              <label class="flex items-center gap-2 text-sm">
                <input type="radio" name="work-arrangement" value="remote" />
                Remote
              </label>
              <label class="flex items-center gap-2 text-sm">
                <input type="radio" name="work-arrangement" value="hybrid" />
                Hybrid
              </label>
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Salary expectations</label>
            <input type="text" class="w-full rounded-sm border border-gray-300 px-3 py-2" placeholder="Negotiable" />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-gray-700">Additional notes</label>
            <textarea class="w-full rounded-sm border border-gray-300 px-3 py-2" rows="3" placeholder="Anything else you would like us to know..."></textarea>
          </div>
        </div>
      </section>

      <section id="review" class="mb-12">
        <h2 class="mb-4 border-b border-gray-200 pb-2 text-xl font-semibold">Review & submit</h2>
        <div class="space-y-4">
          <p class="text-sm text-gray-600"
            >Please review all the information above before submitting. You can click on any section in the table of contents to jump back and make corrections.</p
          >
          <label class="flex items-start gap-2 text-sm">
            <input type="checkbox" class="mt-0.5" />
            <span>I confirm that all information provided is accurate and complete to the best of my knowledge.</span>
          </label>
          <label class="flex items-start gap-2 text-sm">
            <input type="checkbox" class="mt-0.5" />
            <span>I agree to the processing of my personal data in accordance with the privacy policy.</span>
          </label>
          <div class="pt-4">
            <button class="rounded-sm bg-primary-600 px-8 py-3 font-medium text-white hover:bg-primary-700">Submit application</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
